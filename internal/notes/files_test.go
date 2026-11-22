package notes_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// attach sends contents to note as the file name of the type kind, and
// returns the file it answers.
func (f *fixture) attach(t *testing.T, note object, name, kind string, contents []byte, auth string) object {
	t.Helper()

	code, got := f.upload(t, note, name, kind, contents, auth)
	require.Equal(t, http.StatusCreated, code, got)

	return got
}

// upload sends contents to note as the file name of the type kind, and
// decodes what it answers.
func (f *fixture) upload(t *testing.T, note object, name, kind string, contents []byte, auth string) (int, object) {
	t.Helper()

	rec := f.send(t, http.MethodPost, path(note)+"/files?name="+url.QueryEscape(name), contents, kind, auth)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got object
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), rec.Body.String())

	return rec.Code, got
}

func filePath(note, file object) string {
	return fmt.Sprintf("%s/files/%v", path(note), file["id"])
}

// sum is the name contents are kept under.
func sum(contents []byte) string {
	s := sha256.Sum256(contents)

	return hex.EncodeToString(s[:])
}

// kept gives the names under files/ in the data directory, and wants each to
// be the SHA-256 of what it holds and nothing to be left under tmp/.
func (f *fixture) kept(t *testing.T) []string {
	t.Helper()

	list := func(dir string) []os.DirEntry {
		entries, err := os.ReadDir(filepath.Join(f.tempDir, dir))
		if !errors.Is(err, fs.ErrNotExist) {
			require.NoError(t, err)
		}

		return entries
	}

	assert.Empty(t, list("tmp"), "no upload is left under tmp/")

	out := []string{}

	for _, e := range list("files") {
		b, err := os.ReadFile(filepath.Join(f.tempDir, "files", e.Name()))
		require.NoError(t, err)
		assert.Equal(t, sum(b), e.Name(), "a file is named by the SHA-256 of what it holds")

		out = append(out, e.Name())
	}

	return out
}

func TestAFileReadsBackAsItWasSent(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")

	seeds := f.create(t, `{"title":"Seeds","area":"garden"}`, f.laptop)
	assert.Equal(t, []any{}, seeds["files"])
	assert.InDelta(t, 0, seeds["file_count"], 0)

	var (
		files = []any{}
		sums  []string
	)

	const download = "application/octet-stream"

	for _, tt := range []struct {
		name, kind, stored, served, disposition string
		contents                                []byte
	}{
		{"photo.png", "image/png", "image/png", "image/png", "inline", []byte("\x89PNG\r\n\x1a\n\x00\x01\xfe\xff")},
		{"photo.jpg", "IMAGE/JPEG", "image/jpeg", "image/jpeg", "inline", []byte("\xff\xd8\xff\xe0 a jpeg")},
		{"page.html", "text/html; charset=utf-8", "text/html", download, "attachment", []byte("<script>alert(1)</script>")},
		{"drawing.svg", "image/svg+xml", "image/svg+xml", download, "attachment", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{"app.js", "text/javascript", "text/javascript", download, "attachment", []byte("fetch('/api/tokens')")},
		{"style.css", "text/css; charset=utf-8", "text/css", download, "attachment", []byte("body { display: none }")},
		{"notes", "", download, download, "attachment", []byte("<html><script>alert(1)</script>")},
		{"not a type.txt", "text", download, download, "attachment", []byte("plain")},
		{"empty.txt", "text/plain", "text/plain", download, "attachment", nil},
	} {
		got := f.attach(t, seeds, tt.name, tt.kind, tt.contents, session)
		assert.Equal(t, object{"id": got["id"], "name": tt.name, "type": tt.stored, "size": float64(len(tt.contents))}, got,
			"the JSON keeps the type declared")

		rec := f.send(t, http.MethodGet, filePath(seeds, got), nil, "", session)
		require.Equal(t, http.StatusOK, rec.Code, tt.name)
		assert.Equal(t, string(tt.contents), rec.Body.String(), "byte for byte")

		h := rec.Header()
		assert.Equal(t, tt.served, h.Get("Content-Type"))
		assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
		assert.Equal(t, "sandbox", h.Get("Content-Security-Policy"))
		assert.Equal(t, "private, max-age=31536000, immutable", h.Get("Cache-Control"))
		assert.Equal(t, "same-origin", h.Get("Cross-Origin-Resource-Policy"))

		disposition, params, err := mime.ParseMediaType(h.Get("Content-Disposition"))
		require.NoError(t, err)
		assert.Equal(t, tt.disposition, disposition, tt.name)
		assert.Equal(t, map[string]string{"filename": tt.name}, params)

		files, sums = append(files, got), append(sums, sum(tt.contents))
	}

	read := f.read(t, seeds, f.phone)
	assert.Equal(t, files, read["files"], "in the order they were attached")
	assert.InDelta(t, len(files), read["file_count"], 0)
	assert.ElementsMatch(t, sums, f.kept(t))

	for _, key := range []string{"revision", "changed_at", "source"} {
		assert.Equal(t, seeds[key], read[key], "attaching leaves %s alone", key)
	}

	assert.Equal(t, []string{"1 laptop <nil>"}, f.history(t, seeds, f.phone), "attaching writes no revision")

	code, got := f.do(t, http.MethodGet, "/api/notes?area=garden", "", f.phone)
	require.Equal(t, http.StatusOK, code)
	assert.InDelta(t, len(files), got["notes"].([]any)[0].(object)["file_count"], 0)

	code, got = f.do(t, http.MethodDelete, filePath(seeds, files[0].(object)), "", session)
	require.Equal(t, http.StatusNoContent, code, got)

	read = f.read(t, seeds, f.phone)
	assert.Equal(t, files[1:], read["files"])

	for _, key := range []string{"revision", "changed_at", "source"} {
		assert.Equal(t, seeds[key], read[key], "removing leaves %s alone", key)
	}

	code, got = f.do(t, http.MethodGet, path(seeds)+"/files/9999", "", session)
	refused(t, http.StatusNotFound, code, got)
}

func TestOnlyASessionReachesAFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")
	seeds := f.create(t, `{"title":"Seeds"}`, f.laptop)
	kept := f.attach(t, seeds, "seeds.txt", "text/plain", []byte("Tomatoes"), session)

	for _, token := range []string{f.laptop, f.phone} {
		for _, r := range []struct{ method, target string }{
			{http.MethodPost, path(seeds) + "/files?name=more.txt"},
			{http.MethodGet, filePath(seeds, kept)},
			{http.MethodDelete, filePath(seeds, kept)},
		} {
			code, got := f.do(t, r.method, r.target, "More", token)
			refused(t, http.StatusForbidden, code, got)
		}
	}

	code, got := f.do(t, http.MethodGet, filePath(seeds, kept), "", "")
	refused(t, http.StatusUnauthorized, code, got)

	assert.Equal(t, []any{kept}, f.read(t, seeds, f.phone)["files"], "no token attached or removed a file")
	assert.Equal(t, []string{sum([]byte("Tomatoes"))}, f.kept(t))
}

// cut is a body that sends part of a file and then drops, ending the
// request's context as a connection that drops does.
type cut struct {
	cancel context.CancelFunc
	sent   bool
}

func (c *cut) Read(p []byte) (int, error) {
	if !c.sent {
		c.sent = true

		return copy(p, "the first part"), nil
	}

	c.cancel()

	return 0, io.ErrUnexpectedEOF
}

func TestAnUploadThatFailsLeavesNothingBehind(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	code, got := f.do(t, http.MethodGet, "/api/notes/limits", "", "")
	refused(t, http.StatusUnauthorized, code, got)

	code, got = f.do(t, http.MethodGet, "/api/notes/limits", "", f.phone)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, object{"file_max_bytes": float64(15 << 20)}, got, "15 MiB unless set")

	f = newDatabase(t)
	f.load(t, object{"enabled": true, "file_max_mib": int64(1)})
	session := f.signIn(t, "ada")
	seeds := f.create(t, `{"title":"Seeds"}`, f.laptop)

	code, got = f.do(t, http.MethodGet, "/api/notes/limits", "", f.phone)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, object{"file_max_bytes": float64(1 << 20)}, got)

	limit := int(got["file_max_bytes"].(float64))

	code, got = f.upload(t, seeds, "big.bin", "application/octet-stream", make([]byte, limit+1), session)
	refused(t, http.StatusRequestEntityTooLarge, code, got)
	assert.Zero(t, f.count(t, "note_files"))
	assert.Empty(t, f.kept(t))

	ctx, cancel := context.WithCancel(t.Context())
	rec := f.serve(httptest.NewRequestWithContext(ctx, http.MethodPost, path(seeds)+"/files?name=photo.jpg", &cut{cancel: cancel}), session)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	refused(t, http.StatusBadRequest, rec.Code, got)
	assert.Zero(t, f.count(t, "note_files"), "nor one cut short")
	assert.Empty(t, f.kept(t))

	f.attach(t, seeds, "big.bin", "application/octet-stream", make([]byte, limit), session)
	assert.Equal(t, []string{sum(make([]byte, limit))}, f.kept(t), "the cap itself is kept")
}

func TestContentsAreKeptUntilTheLastAttachmentGoes(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")
	seeds := f.create(t, `{"title":"Seeds"}`, f.laptop)
	beds := f.create(t, `{"title":"Beds"}`, f.laptop)
	photo := []byte("\x89PNG the same photo")

	detach := func(note, file object) {
		t.Helper()

		code, got := f.do(t, http.MethodDelete, filePath(note, file), "", session)
		require.Equal(t, http.StatusNoContent, code, got)
	}

	first := f.attach(t, seeds, "first.png", "image/png", photo, session)
	second := f.attach(t, seeds, "second.png", "image/png", photo, session)
	third := f.attach(t, beds, "third.png", "image/png", photo, session)
	assert.Equal(t, []string{sum(photo)}, f.kept(t), "the same contents are kept once")

	detach(seeds, first)
	assert.Equal(t, []string{sum(photo)}, f.kept(t))

	code, got := f.do(t, http.MethodDelete, filePath(seeds, first), "", session)
	refused(t, http.StatusNotFound, code, got)

	code, got = f.do(t, http.MethodDelete, filePath(seeds, third), "", session)
	refused(t, http.StatusNotFound, code, got) // a file of another note

	f.trash(t, beds, "/trash", f.laptop)

	code, got = f.upload(t, beds, "fourth.png", "image/png", photo, session)
	refused(t, http.StatusConflict, code, got)

	code, got = f.do(t, http.MethodDelete, filePath(beds, third), "", session)
	refused(t, http.StatusConflict, code, got)

	rec := f.send(t, http.MethodGet, filePath(beds, third), nil, "", session)
	require.Equal(t, http.StatusOK, rec.Code, "a note in the trash still gives its files")
	assert.Equal(t, string(photo), rec.Body.String())

	code, got = f.do(t, http.MethodDelete, path(beds), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, []string{sum(photo)}, f.kept(t), "while another attachment names them")

	detach(seeds, second)
	assert.Empty(t, f.kept(t), "the last attachment takes them with it")
	assert.Zero(t, f.count(t, "note_files"))

	shed := f.create(t, `{"title":"Shed"}`, f.laptop)
	f.attach(t, shed, "shed.png", "image/png", photo, session)
	f.trash(t, shed, "/trash", f.laptop)

	code, got = f.do(t, http.MethodDelete, path(shed), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Empty(t, f.kept(t), "deleting the note of the last attachment for good takes them with it")

	plans, list := []byte("Plans"), []byte("List")
	planned := f.attach(t, seeds, "plans.txt", "text/plain", plans, session)
	assert.Greater(t, planned["id"], third["id"], "an id is never given again")

	listed := f.create(t, `{"title":"List"}`, f.laptop)
	f.attach(t, listed, "list.txt", "text/plain", list, session)
	f.attach(t, listed, "plans.txt", "text/plain", plans, session)
	f.trash(t, seeds, "/trash", f.laptop)

	code, got = f.do(t, http.MethodDelete, "/api/notes/trash", "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.ElementsMatch(t, []string{sum(list), sum(plans)}, f.kept(t), "emptying the trash keeps what a note outside it names")

	f.trash(t, listed, "/trash", f.laptop)

	code, got = f.do(t, http.MethodDelete, "/api/notes/trash", "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Empty(t, f.kept(t))
}

func TestAFileCrossesNoUser(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada, grace := f.signIn(t, "ada"), f.signIn(t, "grace")
	mine := f.create(t, `{"title":"Seeds"}`, f.laptop)
	theirs := f.create(t, `{"title":"Orchard"}`, f.grace)
	photo := []byte("\x89PNG a photo both keep")

	hers := f.attach(t, mine, "seeds.png", "image/png", photo, ada)
	other := f.attach(t, theirs, "orchard.png", "image/png", photo, grace)
	assert.Equal(t, []string{sum(photo)}, f.kept(t))

	for _, target := range []string{filePath(mine, hers), filePath(theirs, hers)} {
		rec := f.send(t, http.MethodGet, target, nil, "", grace)
		assert.Equal(t, http.StatusNotFound, rec.Code, target)
		assert.NotContains(t, rec.Body.String(), string(photo))

		code, got := f.do(t, http.MethodDelete, target, "", grace)
		refused(t, http.StatusNotFound, code, got)
	}

	code, got := f.upload(t, mine, "lost.png", "image/png", photo, grace)
	refused(t, http.StatusNotFound, code, got)
	assert.Equal(t, []any{hers}, f.read(t, mine, f.laptop)["files"])

	code, got = f.do(t, http.MethodDelete, filePath(mine, hers), "", ada)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, []string{sum(photo)}, f.kept(t), "her removal leaves grace's")

	rec := f.send(t, http.MethodGet, filePath(theirs, other), nil, "", grace)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, string(photo), rec.Body.String())

	code, got = f.do(t, http.MethodDelete, filePath(theirs, other), "", grace)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Empty(t, f.kept(t))
}

func TestAFileIsNamedPlainly(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")
	seeds := f.create(t, `{"title":"Seeds"}`, f.laptop)
	long := strings.Repeat("ø", 127) + "a" // 255 bytes

	for _, tt := range []struct{ given, stored string }{
		{`../../etc/pass"wd`, `pass"wd`},
		{`C:\Users\ada\Seeds; "list".txt`, `Seeds; "list".txt`},
		{"blåbær\r\n\x7f\xff.txt", "blåbær.txt"},
		{long, long},
	} {
		got := f.attach(t, seeds, tt.given, "text/plain", []byte(tt.stored), session)
		assert.Equal(t, tt.stored, got["name"])

		rec := f.send(t, http.MethodGet, filePath(seeds, got), nil, "", session)
		require.Equal(t, http.StatusOK, rec.Code)

		_, params, err := mime.ParseMediaType(rec.Header().Get("Content-Disposition"))
		require.NoError(t, err, "the name leaves the header whole")
		assert.Equal(t, map[string]string{"filename": tt.stored}, params)
	}

	for _, given := range []string{"", "notes/", "..", "\x00\n", long + "a"} {
		code, got := f.upload(t, seeds, given, "text/plain", []byte("Lost"), session)
		refused(t, http.StatusBadRequest, code, got)
	}

	rec := f.send(t, http.MethodPost, path(seeds)+"/files", []byte("Lost"), "text/plain", session)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a file without ?name=")

	assert.Len(t, f.read(t, seeds, f.phone)["files"], 4)
	assert.Len(t, f.kept(t), 4)
}

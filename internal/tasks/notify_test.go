package tasks_test

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
)

// rang is a message bell was asked to deliver, and the user it was for.
type rang struct {
	user int64
	module.Message
}

// at runs the job of notifications at when, "YYYY-MM-DD HH:MM" with or
// without ":SS", in the fixture's zone, as run does.
func (f *fixture) at(t *testing.T, when string) []rang {
	t.Helper()

	at, err := time.ParseInLocation(time.DateTime[:len(when)], when, f.zone)
	require.NoError(t, err)

	return f.run(t, at)
}

// run sets the clock to when, runs the job of notifications once, and gives
// what it handed bell: each user's in the order handed, ada's first.
func (f *fixture) run(t *testing.T, when time.Time) []rang {
	t.Helper()

	f.clock.Store(when.UnixMilli())
	f.rang = nil
	require.NoError(t, f.notify(t.Context()))

	got := f.rang
	slices.SortStableFunc(got, func(a, b rang) int { return cmp.Compare(a.user, b.user) })

	return got
}

func (f *fixture) graceID(t *testing.T) int64 {
	t.Helper()

	var id int64
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT id FROM users WHERE name = 'grace'`).Scan(&id))

	return id
}

// numbers are the whole numbers s holds, in order.
func numbers(s string) []int {
	out := []int{}

	for _, n := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsDigit(r) }) {
		v, err := strconv.Atoi(n)
		if err == nil {
			out = append(out, v)
		}
	}

	return out
}

// reminds checks m reminds user of task at the time at, beside its area's
// name unless area is "".
func reminds(t *testing.T, m rang, user int64, task object, at, area string) {
	t.Helper()

	id := fmt.Sprint(task["id"])

	assert.Equal(t, user, m.user)
	assert.Equal(t, task["title"], m.Title)
	assert.Equal(t, "/tasks/"+id, m.URL)
	assert.Equal(t, "tasks-"+id, m.Tag)

	if area == "" {
		assert.Equal(t, at, m.Body, "the time alone in the personal area")
	} else {
		assert.Regexp(t, `^`+at+`\D.*\b`+area+`$`, m.Body)
	}
}

// tells checks m tells user, with tag and path, the counts in its title and
// the lines in its text.
func tells(t *testing.T, m rang, user int64, tag, path string, counts []int, lines ...string) {
	t.Helper()

	assert.Equal(t, user, m.user)
	assert.Equal(t, tag, m.Tag)
	assert.Equal(t, path, m.URL)
	assert.Equal(t, counts, numbers(m.Title), m.Title)
	assert.Equal(t, lines, strings.Split(m.Body, "\n"))
}

func TestAStartTimeRemindsOnceAtIt(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	grace := f.graceID(t)

	made := f.create(t, `{"tasks":[
		{"title":"Ring the carpenter","planned_from":"2026-11-24","planned_time":"14:00"},
		{"title":"Water the beds","area":"garden","planned_from":"2026-11-24","planned_to":"2026-11-26","planned_time":"14:00"},
		{"title":"Paint","planned_from":"2026-11-24","planned_time":"14:00"},
		{"title":"Sand","planned_from":"2026-11-24","planned_time":"14:00"},
		{"title":"Stain","planned_from":"2026-11-23","planned_to":"2026-11-25","planned_time":"14:00"},
		{"title":"Oil","planned_from":"2026-11-25","planned_time":"14:00"},
		{"title":"Varnish","planned_from":"2026-11-24","planned_time":"14:00"},
		{"title":"Sweep","planned_from":"2026-11-24","planned_time":"14:00"}]}`, f.laptop)
	ring, water, done, trashed, varnish, sweep := made[0], made[1], made[2], made[3], made[6], made[7]
	cat := f.create(t, `{"tasks":[{"title":"Feed the cat","planned_from":"2026-11-24","planned_time":"14:00"}]}`, f.grace)[0]

	f.change(t, done, `{"done":true}`, f.laptop)
	f.trash(t, trashed, "/trash", f.laptop)
	f.change(t, varnish, `{"planned_time":"15:00"}`, f.laptop)

	f.at(t, "2026-11-24 08:00")
	assert.Empty(t, f.at(t, "2026-11-24 13:59:30"))

	got := f.at(t, "2026-11-24 14:00")
	require.Len(t, got, 4, "neither done nor trashed, nor begun another day")
	reminds(t, got[0], f.ada, ring, "14:00", "")
	reminds(t, got[1], f.ada, water, "14:00", "Garden")
	reminds(t, got[2], f.ada, sweep, "14:00", "")
	reminds(t, got[3], grace, cat, "14:00", "")

	assert.Empty(t, f.at(t, "2026-11-24 14:00:30"), "once")

	f.change(t, sweep, `{"planned_time":"15:00"}`, f.laptop)

	got = f.at(t, "2026-11-24 15:00")
	require.Len(t, got, 2, "a time moved to later, before or after it came")
	reminds(t, got[0], f.ada, varnish, "15:00", "")
	reminds(t, got[1], f.ada, sweep, "15:00", "")
}

func TestTheMorningTellsWhatTheDayHolds(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	grace := f.graceID(t)

	made := f.create(t, `{"tasks":[
		{"title":"Ring the carpenter","planned_from":"2026-11-24","planned_time":"14:00"},
		{"title":"Sweep the yard","planned_from":"2026-11-24"},
		{"title":"Mow","planned_from":"2026-11-20","planned_to":"2026-11-21"},
		{"title":"Pay rent","deadline":"2026-11-24"},
		{"title":"Paint the fence","area":"garden","planned_from":"2026-11-24","planned_time":"09:30"},
		{"title":"File taxes","deadline":"2026-11-23"},
		{"title":"Prune","planned_from":"2026-11-22","planned_to":"2026-11-23","deadline":"2026-11-22"},
		{"title":"Stain the deck","planned_from":"2026-11-23","planned_to":"2026-11-25"},
		{"title":"Repot","planned_from":"2026-11-26","deadline":"2026-11-20"},
		{"title":"Clean the gutters","planned_from":"2026-11-10"},
		{"title":"Sort the shed"},
		{"title":"Buy seeds","list":"soon"},
		{"title":"Next week","planned_from":"2026-11-30","deadline":"2026-12-01"},
		{"title":"Done","planned_from":"2026-11-24"},
		{"title":"Trashed","deadline":"2026-11-23"}]}`, f.laptop)
	f.change(t, made[13], `{"done":true}`, f.laptop)
	f.trash(t, made[14], "/trash", f.laptop)

	feed := f.create(t, `{"tasks":[{"title":"Feed the cat","planned_from":"2026-11-24"},{"title":"Brush the cat"},{"title":"Comb the cat","list":"soon"}]}`,
		f.grace)[0]

	f.at(t, "2026-11-24 06:00")
	assert.Empty(t, f.at(t, "2026-11-24 06:59"))

	got := f.at(t, "2026-11-24 07:00")
	require.Len(t, got, 2)
	tells(t, got[0], f.ada, "tasks-morning", "/tasks?view=today", []int{5, 3, 2},
		"09:30 Paint the fence", "14:00 Ring the carpenter", "Sweep the yard", "Pay rent", "Stain the deck",
		"Mow", "Prune", "Clean the gutters",
		"File taxes", "Repot")
	tells(t, got[1], grace, "tasks-morning", "/tasks?view=today", []int{1}, "Feed the cat")

	assert.Empty(t, f.at(t, "2026-11-24 07:00:30"), "once")

	f.change(t, feed, `{"done":true}`, f.grace)

	got = f.at(t, "2026-11-25 07:00")
	require.Len(t, got, 1, "grace's day holds nothing: a task to sort and one on a list are no day's")
	assert.Equal(t, f.ada, got[0].user)
}

func TestATimeIsOnTheFirstDayOfItsPlanAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t)

	code, got := f.do(t, http.MethodPatch, "/api/tasks/notify", `{"untimed":true}`, session)
	require.Equal(t, http.StatusOK, code, got)

	made := f.create(t, `{"tasks":[
		{"title":"Stain the deck","planned_from":"2026-11-23","planned_to":"2026-11-25","planned_time":"14:00"},
		{"title":"Ring the carpenter","planned_from":"2026-11-24","planned_to":"2026-11-25","planned_time":"14:00"},
		{"title":"Sweep the yard","planned_from":"2026-11-24"}]}`, f.laptop)

	f.at(t, "2026-11-24 06:00")

	ran := f.at(t, "2026-11-24 07:00")
	require.Len(t, ran, 1)
	tells(t, ran[0], f.ada, "tasks-morning", "/tasks?view=today", []int{3}, "14:00 Ring the carpenter", "Stain the deck", "Sweep the yard")

	ran = f.at(t, "2026-11-24 09:00")
	require.Len(t, ran, 1)
	tells(t, ran[0], f.ada, "tasks-untimed", "/tasks?view=today", []int{2}, "Stain the deck", "Sweep the yard")

	ran = f.at(t, "2026-11-24 14:00")
	require.Len(t, ran, 1, "the task whose plan begins today, and not the one begun the day before")
	reminds(t, ran[0], f.ada, made[1], "14:00", "")
}

func TestTheDeadlinesAndTheUntimedComeWhenSwitchedOn(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	grace := f.graceID(t)
	session := f.signIn(t)

	code, got := f.do(t, http.MethodPatch, "/api/tasks/notify",
		`{"start":false,"morning":false,"deadline":true,"deadline_at":"17:30","untimed":true,"untimed_at":"08:45"}`, session)
	require.Equal(t, http.StatusOK, code, got)

	made := f.create(t, `{"tasks":[
		{"title":"Pay the plumber","deadline":"2026-11-25"},
		{"title":"Send the forms","planned_from":"2026-11-22","deadline":"2026-11-25"},
		{"title":"Renew the lease","deadline":"2026-11-26"},
		{"title":"Sweep the yard","planned_from":"2026-11-24"},
		{"title":"Stain the deck","area":"garden","planned_from":"2026-11-24","planned_to":"2026-11-25"},
		{"title":"Ring the carpenter","planned_from":"2026-11-24","planned_time":"14:00"}]}`, f.laptop)
	renew, stain := made[2], made[4]

	f.create(t, `{"tasks":[{"title":"Take the cat to the vet","deadline":"2026-11-25"},
		{"title":"Feed the cat","planned_from":"2026-11-24","planned_time":"14:00"}]}`, f.grace)

	f.at(t, "2026-11-24 06:00")

	ran := f.at(t, "2026-11-24 07:00")
	require.Len(t, ran, 1, "her own morning, and not ada's, switched off")
	tells(t, ran[0], grace, "tasks-morning", "/tasks?view=today", []int{1}, "14:00 Feed the cat")

	ran = f.at(t, "2026-11-24 08:45")
	require.Len(t, ran, 1, "at her hour")
	tells(t, ran[0], f.ada, "tasks-untimed", "/tasks?view=today", []int{2}, "Sweep the yard", "Stain the deck")
	assert.Empty(t, f.at(t, "2026-11-24 09:00"), "and not at the first one")

	ran = f.at(t, "2026-11-24 14:00")
	require.Len(t, ran, 1, "her own start time, and not ada's, switched off")
	assert.Equal(t, grace, ran[0].user)
	assert.Equal(t, "Feed the cat", ran[0].Title)

	ran = f.at(t, "2026-11-24 17:30")
	require.Len(t, ran, 1, "ada's, and not grace's, who kept the kind off")
	tells(t, ran[0], f.ada, "tasks-deadline", "/tasks?view=week", []int{2}, "Pay the plumber", "Send the forms")
	assert.Empty(t, f.at(t, "2026-11-24 18:00"))

	assert.Len(t, f.at(t, "2026-11-25 07:00"), 1)

	ran = f.at(t, "2026-11-25 08:45")
	require.Len(t, ran, 1)
	tells(t, ran[0], f.ada, "tasks-untimed", fmt.Sprintf("/tasks/%v", stain["id"]), []int{1}, "Stain the deck")

	ran = f.at(t, "2026-11-25 17:30")
	require.Len(t, ran, 1)
	tells(t, ran[0], f.ada, "tasks-deadline", fmt.Sprintf("/tasks/%v", renew["id"]), []int{1}, "Renew the lease")

	assert.Len(t, f.at(t, "2026-11-26 07:00"), 1)
	assert.Empty(t, f.at(t, "2026-11-26 08:45"), "nothing planned for today with no start time today")
	assert.Empty(t, f.at(t, "2026-11-26 17:30"), "nothing due tomorrow")
}

func TestWhatCameDueWhileTheServerWasDownComesLate(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t)

	code, kinds := f.do(t, http.MethodPatch, "/api/tasks/notify", `{"untimed":true,"untimed_at":"22:00"}`, session)
	require.Equal(t, http.StatusOK, code, kinds)

	f.one(t, `{"title":"Open the shutters","planned_from":"2026-11-24","planned_time":"12:00"}`)
	assert.Empty(t, f.at(t, "2026-11-24 13:00"), "the very first run sends nothing, though a time came before it")

	ring := f.one(t, `{"title":"Ring the carpenter","planned_from":"2026-11-24","planned_time":"14:00"}`)
	f.one(t, `{"title":"Lock up","planned_from":"2026-11-24","planned_time":"22:00"}`)
	f.one(t, `{"title":"Stain the deck","planned_from":"2026-11-24","planned_to":"2026-11-25"}`)

	assert.Empty(t, f.at(t, "2026-11-24 13:50"))
	f.load(t, object{"enabled": true})

	got := f.at(t, "2026-11-24 14:20")
	require.Len(t, got, 1)
	reminds(t, got[0], f.ada, ring, "14:00", "")

	assert.Empty(t, f.at(t, "2026-11-24 21:50"))
	f.load(t, object{"enabled": true})
	assert.Empty(t, f.at(t, "2026-11-25 06:00"), "nothing of the day before, its 22:00 reminder nor its 22:00 planned today")

	got = f.at(t, "2026-11-25 07:00")
	require.Len(t, got, 1)
	assert.Equal(t, "tasks-morning", got[0].Tag)

	water := f.one(t, `{"title":"Water the beds","planned_from":"2026-11-25","planned_time":"10:00"}`)

	ctx, cancel := context.WithCancel(t.Context())
	f.cut, f.rang = cancel, nil
	f.clock.Store(time.Date(2026, 11, 25, 10, 0, 0, 0, f.zone).UnixMilli())
	require.ErrorIs(t, f.notify(ctx), context.Canceled)
	require.Len(t, f.rang, 1)
	f.cut = nil

	got = f.at(t, "2026-11-25 10:00:30")
	require.Len(t, got, 1, "a run the stop cut short keeps no moment, so the next sends its span again")
	reminds(t, got[0], f.ada, water, "10:00", "")

	f.one(t, `{"title":"Rake","planned_from":"2026-11-25","planned_time":"11:00"}`)
	f.muted = true
	f.load(t, object{"enabled": true})
	assert.Empty(t, f.at(t, "2026-11-25 11:00"))

	f.muted = false
	f.load(t, object{"enabled": true})
	assert.Empty(t, f.at(t, "2026-11-25 11:01"), "with no module that delivers the moment moves all the same")
}

func TestADayTheClockChangesSendsEachOnce(t *testing.T) {
	t.Parallel()

	oslo, err := time.LoadLocation("Europe/Oslo")
	require.NoError(t, err)

	for _, day := range []string{"2026-03-29", "2026-10-25"} {
		f := newDatabase(t)
		f.zone = oslo
		f.load(t, object{"enabled": true})

		at := func(hour int) time.Time {
			d, err := time.ParseInLocation(time.DateOnly, day, f.zone)
			require.NoError(t, err)

			return d.Add(time.Duration(hour) * time.Hour)
		}

		task := f.one(t, `{"title":"Inside the hour","planned_from":"`+day+`","planned_time":"02:30"}`)
		sent := map[string]int{}

		// Every ten minutes as time passes, whatever the clock on the wall says.
		for when := at(-1); when.Before(at(9)); when = when.Add(10 * time.Minute) {
			for _, m := range f.run(t, when) {
				sent[m.Tag]++
			}
		}

		assert.Equal(t, map[string]int{fmt.Sprintf("tasks-%v", task["id"]): 1, "tasks-morning": 1}, sent, day)
	}
}

func TestTheKindsOfNotificationAreTheSessionsAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t)

	code, got := f.do(t, http.MethodGet, "/api/tasks/notify", "", session)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, object{
		"start": true, "morning": true, "morning_at": "07:00", "deadline": false, "deadline_at": "18:00", "untimed": false, "untimed_at": "09:00",
	}, got, "a user who changed none has the first kinds")

	want := maps.Clone(got)
	want["deadline"], want["untimed_at"] = true, "08:15"

	code, got = f.do(t, http.MethodPatch, "/api/tasks/notify", `{"deadline":true,"untimed_at":"08:15"}`, session)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, want, got, "a change sets only what it gives")

	for _, body := range []string{
		`{"morning_at":"7:00"}`, `{"deadline_at":"25:00"}`, `{"untimed_at":""}`, `{"start":false,"morning_at":"07:00:00"}`,
		`{"start":"yes"}`, `{"evening":true}`, `{"start":false} {}`,
	} {
		code, got := f.do(t, http.MethodPatch, "/api/tasks/notify", body, session)
		refused(t, http.StatusBadRequest, code, got)
	}

	for _, auth := range []string{f.laptop, f.phone} {
		for _, r := range []struct{ method, body string }{{http.MethodGet, ""}, {http.MethodPatch, `{"start":false}`}} {
			code, got := f.do(t, r.method, "/api/tasks/notify", r.body, auth)
			refused(t, http.StatusForbidden, code, got)
		}
	}

	_, got = f.do(t, http.MethodGet, "/api/tasks/notify", "", session)
	assert.Equal(t, want, got, "a refused change changes nothing")
}

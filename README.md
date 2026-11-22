# Recall Synapsis

Recall Synapsis is a self-hosted place for one owner to keep what they know, by
area: a personal area and one per project. A person reaches it through a web app
on desktop and phone, and AI clients reach it over MCP, directly or through an
MCP gateway such as Portal. It is one Go binary with the web app embedded.

- The owner signs in with a name and a password. AI clients and scripts use API
  tokens made in Settings, each with read or write access.
- What it keeps comes from feature modules, each switched on in the
  configuration. This build has one, notes: notes and ideas in the areas, with
  their tags, links, attached files and history, an inbox of captures that wait
  to be filed, and a trash. In the web app, Capture saves a thought and the
  Inbox files it; Notes lists the filed notes, each read, edited, moved to
  another area or kind, tagged, linked, given files, brought back to an earlier
  revision and trashed on its page, which shows what links to it and its
  history; the idea bank lists every filed idea by area; and an area's page
  shows its notes and ideas. The rest is reached through the REST API. An AI
  client saves, lists, reads, changes and trashes notes through the MCP tools
  `create_note`, `list_notes`, `get_note`, `update_note` and `trash_note`, and
  what it saves without naming an area or calling it an idea waits in the inbox.
- Its search, in the web app, at `GET /api/search` and as the MCP tool
  `search`, asks every module switched on. Notes finds the notes that hold
  every word given, in their title, body or tags, and part of a word is
  enough. The MCP tool `list_areas` lists the areas.
- It backs itself up every night, into a directory you name.

## What it needs

- Go, in the version `go.mod` names, and Node, in the version `.nvmrc` names,
  to build it; or Docker, to build the image.
- An https address in front of it, which passes requests on. Recall Synapsis
  serves plain HTTP and does no TLS.

## From a clone to a sign-in

1. Build and install ([Building](docs/running.md#building)):

   ```sh
   make web-install
   make install
   ```

   `make install` puts `recallsynapsis` in `GOBIN`, else in the `bin` directory
   of `go env GOPATH`. Put that directory on your `PATH`.

2. Make the data directory, `~/.local/share/recallsynapsis` unless
   `RECALLSYNAPSIS_DATA_DIR` or `XDG_DATA_HOME` is set
   ([The data directory](docs/configuration.md#the-data-directory)), and write
   `recallsynapsis.toml` in it, with `public_url` set to the https address in
   front and `owner` to the name you sign in with
   ([recallsynapsis.toml](docs/configuration.md#recallsynapsistoml)):

   ```sh
   mkdir -p -m 700 ~/.local/share/recallsynapsis
   cat > ~/.local/share/recallsynapsis/recallsynapsis.toml <<'EOF'
   public_url = "https://recall.example.org"
   owner      = "ada"
   EOF
   ```

3. Set the password, at least 12 characters
   ([The password](docs/running.md#the-password)): type it, press Enter, then
   Ctrl-D.

   ```sh
   sh -c 'umask 077 && cat > password.txt'
   recallsynapsis password < password.txt
   rm password.txt
   ```

4. Start it. It listens on `127.0.0.1:7119`:

   ```sh
   recallsynapsis serve
   ```

5. Put it behind the proxy that gives it its https address
   ([What stands in front](docs/running.md#what-stands-in-front)), and sign in
   there.
6. Make a token in Settings for each client
   ([API tokens](docs/running.md#api-tokens)).
7. Set `backup.dir` for the nightly backups and start `serve` again
   ([Backups](docs/running.md#backups)).

To run it from the image instead, see
[From the image](docs/running.md#from-the-image).

## Documents

- [docs/configuration.md](docs/configuration.md): the data directory and every
  key of `recallsynapsis.toml`.
- [docs/running.md](docs/running.md): building, the verbs, the password, the
  API tokens, the image, what stands in front, backups and restoring, and
  upgrading.

## Contributing

`make check` runs the Go gates and `make web-check` those of the web app;
`make help` lists the targets. Read [AGENTS.md](AGENTS.md) first.

## Licence

MIT, in [LICENSE](LICENSE).

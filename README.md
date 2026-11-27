# Recall Synapsis

Recall Synapsis is a self-hosted place for one owner to keep what they know, by
area: a personal area and one per project. A person reaches it through a web app
on desktop and phone, and AI clients reach it over MCP, directly or through an
MCP gateway such as Portal. It is one Go binary with the web app embedded.

Each area has a brief: Markdown of up to 4,000 characters that says what the
area is, why it exists, what it should do and what shapes it. Every wording of
it is kept, any can be read and restored through the API, and a change made
from a wording that has since been replaced is refused. Only a signed-in
session changes a brief, renames a project or archives one.

- The owner signs in with a name and a password. AI clients and scripts use API
  tokens made in Settings, each with read or write access.
- What it keeps comes from feature modules, each switched on in the
  configuration. This build has three. Notes keeps notes and ideas in the areas,
  with their tags, links, attached files and history, an inbox of captures that
  wait to be filed, and a trash. In the web app, Capture saves a thought and the
  Inbox files it; Notes lists the filed notes, each read, edited, moved to
  another area or kind, tagged, linked, given files, brought back to an earlier
  revision and trashed on its page, which shows what links to it and its
  history; the idea bank lists every filed idea by area; an area's page shows
  its notes and ideas; and Trash lists what was trashed, restores it, and
  deletes it for good, one note or all at once. An AI client saves, lists,
  reads, changes and trashes notes through the MCP tools `create_note`,
  `list_notes`, `get_note`, `update_note` and `trash_note`, and what it saves
  without naming an area or calling it an idea waits in the inbox.
- Tasks keeps tasks in the areas, each with its notes, a planned period of days
  with an optional time on its first, a deadline, the list Soon or Someday, when
  it was done, its links and a log of its changes; and a trash. Its REST API, at
  `/api/tasks`, makes several at once, all or none, lists them by area, list,
  state and a span of dates, tells which day it is in the configured time zone,
  trashes and restores them, and deletes those in the trash for good, one or all
  at once. An AI client makes, lists, reads, changes and trashes tasks through
  the MCP tools `create_tasks`, `list_tasks`, `get_task`, `update_task` and
  `trash_task`, and a call that sets a date names the day the client takes for
  today, and is refused with the right one when it is wrong. In the web app
  Tasks is the landing page, its entry first in the navigation with the count of
  what waits to be sorted. Tasks opens on Agenda, where each open task appears
  once. Scheduled holds tasks with a plan or deadline, and Unscheduled holds
  the rest. Today holds what waits to be sorted, what was left from earlier,
  what is overdue and what today holds. Week shows the rest of the week and
  what lies beyond it. Soon and Someday show their lists. Finished tasks
  appear in Done, newest first, grouped by the day they were done. Each
  nonempty view shows its count for the selected area. Views that do not fit
  beside the current view are under More. Capture also makes a task from its
  first line as the title and the rest as notes, in the area chosen, planned by a shortcut
  or left to be sorted. Make a task, on a note's or an idea's page, makes one
  with the fields of a task's form and a link back to the note, which stays
  and lists it; in the Inbox it makes one to sort at once, of the capture's
  text, in the area chosen and with what the owner said, and then moves the
  capture to the trash. A task is added where one stands, and opens
  beside the list, or as a page on a phone, where it is read first and edited in
  one form that saves nothing before Save: its title, notes, plan, with
  shortcuts for today, tomorrow, the weekend and next week, deadline, list and
  area. Its ring, on its row or its page, marks it done at once, its Trash moves
  it to the trash at once, a link is added or removed at once on its page, which
  shows what it links to and what links to it, and its log tells each change,
  with what the owner said. Trashed, at the head of Tasks, lists the trash, the
  most recently trashed first, where a task is read only, restored, and deleted
  for good, one or all at once. An area's page shows its open tasks first,
  grouped by date and list, under a line that adds one to it unless it is
  archived, beside its notes and ideas, and leads to what was done in it. Search
  lists the tasks' hits before the notes', and a task found there or linked from
  a note shows its ring and leads to its page.
- Push sends a module's notifications as Web Push to every browser of the
  owner's in which they are turned on, encrypted for each browser, and a
  browser is turned on or off in Settings, under Notifications, with the
  session cookie alone. Tasks sends four kinds, of the open tasks: a reminder
  at a task's start time, and what the day holds at a morning hour, both on at
  first; and, once switched on, the tasks due tomorrow and those planned for
  today with no start time today, each at its own hour. Each kind is switched,
  and its hour set, in Settings under Notifications too, with the session
  cookie alone: a switch applies the moment it is flipped, and an hour waits
  for Save.
- Its search, in the web app, at `GET /api/search` and as the MCP tool
  `search`, asks every module switched on. Notes finds the notes that hold
  every word given, in their title, body or tags, and Tasks the tasks, done or
  open, that hold them in their title or notes; part of a word is enough. The
  MCP tool `list_areas` lists the areas, each with its brief.
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
  API tokens, the image, what stands in front, notifications, backups and
  restoring, and upgrading.

## Contributing

`make check` runs the Go gates and `make web-check` those of the web app;
`make help` lists the targets. Read [AGENTS.md](AGENTS.md) first.

## Licence

MIT, in [LICENSE](LICENSE).

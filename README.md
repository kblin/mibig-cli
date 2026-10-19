mibig
=====

A command line tool for [MIBiG](https://mibig.secondarymetabolites.org/)
entries, built on [go-mibig](https://github.com/mibig-secmet/go-mibig).


Installation
------------

```sh
go install go.kblin.dev/mibig-cli@latest
```

Editing entries
---------------

```sh
mibig edit BGC0000001.json
```

opens the entry in an interactive editor. The left pane shows the entry as a
tree, the right pane the selected value with its validation issues, and the
bottom pane all issues. The entry is validated after every change.

| Key            | Action                                                       |
|----------------|--------------------------------------------------------------|
| ↑ ↓ / j k      | move                                                         |
| → ← / l h      | expand, collapse / go to parent                              |
| enter          | edit a value, expand/collapse a section                      |
| a              | add a list element (after the selected one), create a section |
| d              | delete an element or unknown key, clear a value              |
| K J            | move a list element up/down                                  |
| e / E          | edit the selected part / the whole entry as JSON in `$EDITOR` |
| n N            | jump to the next/previous issue                              |
| tab            | switch between tree and issue list                           |
| v              | validate at another quality level                            |
| u / ctrl+r     | undo / redo                                                  |
| w / ctrl+s     | save                                                         |
| ?              | show all keys                                                |
| q              | quit                                                         |

Controlled fields, such as evidence methods or biosynthetic subclasses, are
edited by picking from their vocabulary; type `/` to filter. Adding classes,
modules or domains asks for their type first. Keys that are not part of the
schema are shown in the tree and can be deleted or edited as JSON.

Free text such as comments is edited over multiple lines (alt+enter for a new
line). Values that the input fields cannot show exactly, e.g. with tabs or
other control characters, are opened as JSON in your editor instead, so that
they are never changed by accident.

Saving an entry with validation errors asks for confirmation, so that work in
progress can be saved.


Configuration
-------------

Settings are read from flags, `MIBIG_*` environment variables and
`$XDG_CONFIG_HOME/mibig/config.yaml` (default `~/.config/mibig/config.yaml`),
in that order of precedence:

```yaml
# command to edit JSON with; default $VISUAL, $EDITOR or vi
editor: code --wait
edit:
  # quality level to validate at; default the entry's own
  validate-at: high
```

The matching flags are `--editor` and `--validate-at`, the environment
variables `MIBIG_EDITOR` and `MIBIG_EDIT_VALIDATE_AT`.


Development
-----------

```sh
go test ./...
# check the editor's view of every entry of a release
MIBIG_DATA=/path/to/mibig-json go test ./internal/tree -run Dataset
```

License
-------
This project is licensed under the Apache License 2.0.
See the [`LICENSE`](LICENSE) file for details.

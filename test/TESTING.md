# Running tests

From the repository root, run a focused case in its owning package.

```sh
python3 test/test.py --run '^TestHomeDChangesDebugVisibility$' ./game/setupui
python3 test/test.py --debug --run '^TestHandledReportBoundedAndFailureIsolated$' ./game/diagnostics
python3 test/test.py --headless --run '^TestGraphicsModalDoesNotAgeFlightInput$' ./game/menus
```

The driver isolates player data and fails when no selected tests run in a package, or a selected case skips. Run tests in an isolated OS account or disposable HOME when supplying additional integration inputs. Native and headless select different bindings. Linux, Windows and ARM also select platform files. Use `GOWORK=off go test -list . ./...` from `src/` to inspect registration; add `-tags headless`, `-tags devtools`, or both. Scenario CLI coverage belongs to the devtools variant. Discovery does not run assertions.

[test-map.csv](test-map.csv) lists prior and current names and file locations. The same test package retains access to private code. Existing subcases, fixtures and failure assertions travel with the parent. Each focused command should use the current names in the staging_symbol column. Build/package commands are in [README.md](../README.md).

[EXTERNAL-FIXTURES.md](../distrib/EXTERNAL-FIXTURES.md) describes the named fixture root. Tests also support explicit environment paths to compatible prepared assets, standalone films, original data and archived observations. `GS2000_TEST_MISSIONS` supplies prepared assets to the mission tests, `GS2000_R7_FILM` a compatible historical film, and `GS2000_R81_SCENES` the accepted scene descriptions. Names previously used as external configuration keys remain supported. See [fixture environment index](../distrib/fixture-environment.csv) for the owning test. Private films, historical binary/reference data and the commercial original installation are supplied independently. Always provide disposable destinations for output or interruption tests. The shared reference inputs are test-data/weapon-table/rows.json and test-data/render-scenes/scenes.json. The rendering scene loader uses the bundled file when GS2000_R81_SCENES is unset.

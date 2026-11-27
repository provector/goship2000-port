# Historical film and view fixtures

Use disposable copies of the private acceptance inputs. The current player accepts the current replay container. The protected P2/P3 tests hash their historical source before invoking the explicit converter. Other viewer tests receive converted copies and retain their original gameplay checkpoints. Keep the unrecovered film separate from the published recovery witness.

From the repository root, with the established Go toolchain and offline module cache available, run the following against a new output directory. The JSON is the retained test environment with absolute fixture paths, including `GS2000_TEST_GO` and `GS2000_FIXTURE_ROOT`.

```sh
python3 test/retained/prepare.py --fixture-env "$FIXTURE_ENV" --ground "$GROUND_SCENES" --output "$NEW_FIXTURES"
. "$NEW_FIXTURES/fixtures.sh"
cd src
"$GS2000_TEST_GO" test -mod=readonly -count=1 -run '^TestRecoverPreservedFullWitness$' ./game/menus
"$GS2000_TEST_GO" test -mod=readonly -count=1 -tags headless -run '^TestRecoverPreservedFullWitness$' ./game/menus
```

Load the base fixture environment and set private HOME, cache, temp and writable data paths before sourcing the generated overrides. The full selected-case run does this for each test. The named `ground-scenes/pad-scene.json` is verified against the retained Q3.2 capture. The catalogue comparison uses both the R23 acceptance preparation (`GS2000_TEST_R221_ASSETS`) and the complete later preparation (`GS2000_TEST_MISSIONS`).

The historical recovery input needs both the original film and its matching loose receipt. The preparer verifies their digests, demonstrates the current converter refusing separate metadata, then uses the released W3.5 companion migrator on a private copy before conversion. This requires local Git object `154cae101d993a191769d51fe3d120fb4a65a89f`. Only the historical Go library runs. The resulting publication carries the original provenance, cursor, operations, events and ending. The checked-in TSV contains source digests. Distribute the test code without the commercial assets, films or user stores.

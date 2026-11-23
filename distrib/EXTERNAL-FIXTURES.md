# External fixture root

Set GS2000_FIXTURE_ROOT to a directory containing the following named inputs. Supply legally obtained, read-only copies. Private inputs require separate authorization. Each test retains its original validations and prerequisite handling.

- `audio-reference`
- `campaign-continuations`
- `campaign-islands`
- `campaign-preparation`
- `captain-observation`
- `cockpit-raster`
- `combat-vectors`
- `commission-sortie.json`
- `damage`
- `editor-air-options.csv`
- `enemy-operands`
- `flight-reference`
- `flight-return`
- `generation`
- `generic-missions`
- `ground-movement`
- `installation`
- `integrated-training`
- `material-sequence`
- `model-budget`
- `opl-reference`
- `prepared-missions`
- `prepared-preflight`
- `rendering`
- `servicing`
- `source-image.gs2`
- `threat-display`

- `captain-roster.dat`
- `captain-squadron.dat`
- `intro-hashes`
- `intro-palette`
- `office-prelude`
- `office-trace`

Historical office/startup/outfit checks formerly looked underneath GS2000_PROJECT_ROOT. Supply the named inputs above through GS2000_FIXTURE_ROOT instead. Historical checkpoint output additionally requires a disposable GS2000_CHECKPOINT_OUTPUT_ROOT and a new child output directory.

The private moving-ground controls read field-scene.json (field, step 515) and pad-scene.json (pad, step 160) from GS2000_Q32_GROUND_SCENES. Give the corresponding verified scene exports those local names. The retained-prefix and search-document checks compare cryptographic fingerprints of their original embedded IDs. The compressed film, original IDs, and the earned-sortie journal stay with the private fixture owner. An immutable SHA-256 of the journal remains its content check.

The two shipped references live under test-data/weapon-table/ and test-data/render-scenes/. [The test guide](../test/TESTING.md) describes the runner and optional environment keys.

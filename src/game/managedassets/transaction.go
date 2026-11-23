package managedassets

import (
	"encoding/json"
	"errors"
	"fmt"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrOriginalUnavailable = errors.New("original installation unavailable")

type Services struct {
	Discover       func() string
	SourceSelected func(string) error
	Pack           func(string, string) error
	Prepare        func(string, string) (*assetdata.Manifest, error)
	Load           func(string) (*assetdata.OfficeAssets, error)
	Rename         func(string, string) error
	Ready          func(*assetdata.OfficeAssets) error
	Progress       func(assetdata.PreparationProgress)
}

func Ensure(paths configuration.Paths, rebuild bool, log io.Writer, s Services) (*assetdata.OfficeAssets, error) {
	// Reader compatibility is not current UI readiness. This check does not
	// inspect user film/mission lists or require originals on a usable fast path.
	managed := paths.AssetsOrigin == "default-managed" || paths.AssetsOrigin == "configured-default-managed" || paths.AssetsOrigin == "" && filepath.Clean(paths.Assets) == filepath.Join(paths.Directory, "assets")
	load := func(path string) (*assetdata.OfficeAssets, error) {
		if s.Pack != nil && managed {
			if _, err := os.Lstat(filepath.Join(path, "packs.json")); err != nil {
				return nil, fmt.Errorf("managed prepared packs: %w", err)
			}
		}
		a, e := s.Load(path)
		if e == nil && s.Ready != nil {
			e = s.Ready(a)
		}
		return a, e
	}
	emit := func(stage string, n, total int) {
		if s.Progress != nil {
			s.Progress(assetdata.PreparationProgress{Stage: stage, Completed: n, Total: total})
		}
	}
	journal := paths.Assets + ".prepare-recovery.json"
	_, journalErr := os.Lstat(journal)
	var reason error
	if os.IsNotExist(journalErr) && !rebuild {
		if a, e := load(paths.Assets); e == nil {
			diagnostics.Info("assets", "Prepared asset cache reused", "path", paths.Assets)
			return a, nil
		} else {
			reason = e
		}
	}
	// An explicit/external tree is a reference, not an owned generated cache.
	// Even a recognized generator manifest is not consent to replace it.

	if !managed && !rebuild {
		if reason == nil {
			reason = fmt.Errorf("an interrupted preparation needs explicit recovery")
		}
		return nil, fmt.Errorf("selected assets at %s are not ready: %w; this explicitly selected tree was preserved; select a current prepared tree or prepare a new output", paths.Assets, reason)
	}
	input, target, err := ManagedPaths(paths.Original, paths.Assets)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return nil, err
	}
	if err = recoverGeneration(target, log, s.Rename, load); err != nil {
		return nil, err
	}
	if !rebuild {
		if assets, err := load(target); err == nil {
			diagnostics.Info("assets", "Prepared asset cache reused after lock/recovery", "path", target)
			return assets, nil
		} else {
			diagnostics.Info("assets", "Managed cache needs preparation", "path", target, "reason", err)
			fmt.Fprintf(log, "Assets need preparation: %v\n", err)
		}
	} else {
		fmt.Fprintln(log, "Rebuilding prepared assets from configured originals.")
	}
	if err = ReplaceableAssets(target); err != nil {
		return nil, err
	}
	// The parser requires a nonexistent output. Reserve a sibling container, then
	// generate its child tree. Failed/interrupted trees are retained, never deleted.
	staging, err := os.MkdirTemp(filepath.Dir(target), "."+filepath.Base(target)+".preparing-")
	if err != nil {
		return nil, err
	}
	generated := filepath.Join(staging, "tree")
	diagnostics.Info("assets", "Preparing assets from original installation", "input", input, "staging", generated, "rebuild", rebuild)
	fmt.Fprintf(log, "Preparing assets from %s ...\n", input)
	// The first progress event opens/raises the small native progress window.
	// Lock/recovery checks may still reuse a cache, so publish only once actual
	// processing is about to start. Missing-input errors use the startup dialog.

	var manifest *assetdata.Manifest
	if s.Pack != nil && managed && !rebuild {
		if _, pe := os.Lstat(filepath.Join(target, "packs.json")); os.IsNotExist(pe) {
			if data, re := assetdata.ReadLimitedFile(filepath.Join(target, "manifest.json"), 64<<20); re == nil {
				var candidate assetdata.Manifest
				if json.Unmarshal(data, &candidate) == nil && (candidate.PreparationVersion == 34 || candidate.PreparationVersion == assetdata.PreparationVersion) {
					emit("Pack existing prepared assets", 0, 1)
					if pe = s.Pack(target, generated); pe == nil {
						manifest = &candidate
					} else {
						fmt.Fprintf(log, "Loose generation could not be upgraded: %v; preparing from originals.\n", pe)
						generated = filepath.Join(staging, "replacement")
					}
				}
			}
		}
	}
	loose := ""
	selected := ""
	if manifest == nil {
		if s.Discover != nil {
			configured := paths.Config.Original
			if configured == "" {
				configured = paths.Original
			}
			candidate, found := selectOriginal(paths.Directory, configured, s.Discover)
			if !found {
				return nil, missingOriginal(configuration.Relative(paths.Directory, candidate), paths.Directory)
			}
			if candidate != configured {
				selected = candidate
				input, _, err = ManagedPaths(configuration.Relative(paths.Directory, candidate), target)
				if err != nil {
					return nil, err
				}
			}
		}
		info, err := os.Stat(input)
		if err != nil || !info.IsDir() {
			return nil, missingOriginal(input, paths.Directory)
		}
		emit("Prepare assets from original installation", 0, 0)
		output := generated
		if s.Pack != nil {
			loose = filepath.Join(staging, "conversion")
			output = loose
		}
		diagnostics.Info("assets", "Selected installation for conversion", "original", input, "staging", output, "destination", target)
		manifest, err = s.Prepare(input, output)
		if manifest != nil {
			diagnostics.Info("assets", "Preparation manifest summary", "version", manifest.PreparationVersion, "result", manifest.Result, "inputs", len(manifest.Resources), "counts", manifest.Summary, "unsupported_operations", manifest.UnsupportedVersionOperations, "fatal", manifest.FatalError)
		}
		if err == nil && s.Pack != nil {
			emit("Create and validate asset packs", 0, 1)
			err = s.Pack(loose, generated)
		}
		if err != nil {
			var details []string
			if manifest != nil {
				for _, op := range manifest.UnsupportedVersionOperations {
					details = append(details, op.Operation+": "+op.Reason)
				}
			}
			if len(details) > 0 {
				err = fmt.Errorf("%w; preparation manifest: %s", err, strings.Join(details, "; "))
			}
			return nil, fmt.Errorf("preparing %s for %s (attempt retained at %s): %w", input, target, staging, err)
		}
	}
	// Partial conversion includes unrelated unsupported formats/PHIL3MIS. Office
	// readiness is determined by verified required outputs, never that aggregate.
	emit("Validate current prepared capabilities", 0, 1)
	if _, err = load(generated); err != nil {
		// Partial raw extraction remains useful, but normal startup must expose the
		// generator's actual unsupported identity rather than only missing UI.
		details := ""
		if manifest != nil && len(manifest.UnsupportedVersionOperations) != 0 {
			var causes []string
			for _, unsupported := range manifest.UnsupportedVersionOperations {
				causes = append(causes, unsupported.Operation+": "+unsupported.Reason)
			}
			details = "; unsupported source profile: " + strings.Join(causes, "; ")
		}
		return nil, fmt.Errorf("generated office assets unusable at %s: %w%s; previous assets unchanged; check supported source profile and configured original path", staging, err, details)
	}
	fmt.Fprintf(log, "Prepared %d entries, %d pictures; %d unsupported, %d conversion failures (office ready).\n", manifest.Summary.Entries, manifest.Summary.Pictures, manifest.Summary.Unsupported, manifest.Summary.ConversionFailures)
	if manifest.Summary.ConversionFailures > 0 {
		diagnostics.Warn("assets", "Preparation has conversion failures; current frontend readiness validated", "conversion_failures", manifest.Summary.ConversionFailures, "unsupported", manifest.Summary.Unsupported, "outcome", "validated frontend usable; exclusions retained in prepared manifest")
	}
	emit("Publish prepared cache", 0, 1)
	var previous string
	var pending *string
	if s.Pack != nil {
		pending = &previous
	}
	if err = promotePreparation(target, generated, log, s.Rename, pending); err != nil {
		return nil, fmt.Errorf("asset promotion failed; attempt retained at %s: %w", staging, err)
	}
	_ = os.Remove(staging) // Empty reservation only; no recursive cleanup.
	diagnostics.Info("assets", "Prepared cache promoted and validated", "path", target)
	assets, finalErr := load(target)
	if finalErr != nil {
		return nil, fmt.Errorf("activated assets failed final validation: %w; retained predecessor %s", finalErr, previous)
	}
	if s.Pack != nil {
		if previous != "" {
			if err := os.Remove(target + ".prepare-recovery.json"); err != nil {
				return nil, err
			}
		}
		if loose != "" {
			if err := os.RemoveAll(loose); err != nil {
				fmt.Fprintf(log, "Conversion staging retained: %v\n", err)
			}
		}
		if previous != "" {
			if err := retireGenerated(previous); err != nil {
				fmt.Fprintf(log, "Previous tree retained at %s: %v\n", previous, err)
			} else {
				_ = os.Remove(filepath.Dir(previous))
			}
		}
		_ = os.Remove(staging)
	}
	if s.SourceSelected != nil && selected != "" {
		if err := s.SourceSelected(selected); err != nil {
			return nil, fmt.Errorf("prepared assets are ready; saving selected original path: %w", err)
		}
	}
	diagnostics.Info("assets", "DOS SETUP preferences, campaigns and saved missions were skipped. GoShip settings and careers remain in the player data store.")
	emit("Prepared cache ready", 1, 1)
	return assets, nil
}

func missingOriginal(path, directory string) error {
	return fmt.Errorf("%w at %s; local fallback: %s; select Original Installation in SETUP > PORT SETTINGS", ErrOriginalUnavailable, path, filepath.Join(directory, configuration.DefaultOriginal))
}

func ManagedPaths(input, output string) (string, string, error) {
	in, err := assetdata.ResolvedNewPath(input)
	if err != nil {
		return "", "", err
	}
	out, err := assetdata.ResolvedNewPath(output)
	if err != nil {
		return "", "", err
	}
	if assetdata.Within(in, out) || assetdata.Within(out, in) {
		return "", "", fmt.Errorf("original and assets directories must not overlap")
	}
	// Reuse the frame writer's protected-tree/ancestor checks on a nonexistent
	// sibling. This also protects canonical Step A/21 outputs during regeneration.
	if _, err = assetdata.GeneratedPath(filepath.Join(out, ".preparation-safety-check"), in); err != nil {
		return "", "", err
	}
	if original, err := os.Lstat(output); err == nil && original.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("refusing to replace symlink asset directory %s", output)
	}
	return in, out, nil
}

func ReplaceableAssets(directory string) error {
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("assets target must be an ordinary directory: %s", directory)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	data, err := assetdata.ReadLimitedFile(filepath.Join(directory, "manifest.json"), 64<<20)
	var manifest assetdata.Manifest
	if err == nil {
		err = json.Unmarshal(data, &manifest)
	}
	// A legacy Step21 tree can be reused, but automatic replacement requires the
	// explicit generator identity. Moving an arbitrary folder is not a rebuild.
	if err != nil || manifest.Generator != assetdata.PreparationGenerator || manifest.PreparationVersion < 1 || manifest.SchemaVersion != assetdata.PreparedManifestSchema {
		return fmt.Errorf("refusing to replace unrelated or unidentified nonempty directory %s; use a new assets path (legacy Step21 trees remain readable)", directory)
	}
	return nil
}

type preparationRecovery struct {
	Version int    `json:"version"`
	Target  string `json:"target"`
	Backup  string `json:"backup_container"`
}

func PromotePreparation(target, generated string, log io.Writer, rename func(string, string) error) error {
	return promotePreparation(target, generated, log, rename, nil)
}
func promotePreparation(target, generated string, log io.Writer, rename func(string, string) error, previous *string) error {
	if err := ReplaceableAssets(target); err != nil {
		return err
	}
	_, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return rename(generated, target)
	}
	if err != nil {
		return err
	}
	backup, err := os.MkdirTemp(filepath.Dir(target), "."+filepath.Base(target)+".previous-")
	if err != nil {
		return err
	}
	journal := target + ".prepare-recovery.json"
	version := 1
	if previous != nil {
		version = 2
	}
	record := preparationRecovery{version, filepath.Base(target), filepath.Base(backup)}
	data, _ := assetdata.JsonBytes(record)
	// Persist the recovery pointer before moving the old tree. The two renames
	// are portable operations, not a claim of universally atomic replacement.
	f, err := os.OpenFile(journal, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	old := filepath.Join(backup, "tree")
	if err = rename(target, old); err != nil {
		_ = os.Remove(journal)
		return err
	}
	if err = rename(generated, target); err != nil {
		if restoreErr := rename(old, target); restoreErr != nil {
			return fmt.Errorf("promotion failed: %v; restoration failed: %v; previous tree retained at %s; recovery journal %s", err, restoreErr, old, journal)
		}
		_ = os.Remove(journal)
		_ = os.Remove(backup)
		return fmt.Errorf("promotion failed; previous assets restored, generated attempt retained at %s: %w", generated, err)
	}
	if previous == nil {
		if err = os.Remove(journal); err != nil {
			return fmt.Errorf("assets promoted; recovery journal removal failed: %w", err)
		}
	}
	if previous != nil {
		*previous = old
		fmt.Fprintf(log, "Previous generation held for validation at %s\n", old)
	} else {
		fmt.Fprintf(log, "Previous asset tree retained at %s\n", old)
	}
	return nil
}

func RecoverPreparation(target string, log io.Writer, rename func(string, string) error) error {
	return recoverGeneration(target, log, rename, nil)
}
func recoverGeneration(target string, log io.Writer, rename func(string, string) error, load func(string) (*assetdata.OfficeAssets, error)) error {
	filename := target + ".prepare-recovery.json"
	data, err := assetdata.ReadLimitedFile(filename, 4096)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var record preparationRecovery
	if err = assetdata.DecodeObject(data, &record); err != nil {
		return fmt.Errorf("invalid preparation recovery journal %s: %w", filename, err)
	}
	if (record.Version != 1 && record.Version != 2) || record.Target != filepath.Base(target) || filepath.Base(record.Backup) != record.Backup || !strings.HasPrefix(record.Backup, "."+filepath.Base(target)+".previous-") {
		return fmt.Errorf("unsafe preparation recovery journal %s", filename)
	}
	if _, err = os.Lstat(target); os.IsNotExist(err) {
		previous := filepath.Join(filepath.Dir(target), record.Backup, "tree")
		resolved, e := filepath.EvalSymlinks(previous)
		if e != nil || resolved != previous {
			return fmt.Errorf("cannot safely restore previous assets at %s", previous)
		}
		if err = rename(previous, target); err != nil {
			return fmt.Errorf("restore previous assets %s: %w", previous, err)
		}
		fmt.Fprintf(log, "Restored interrupted preparation's previous tree from %s\n", previous)
		diagnostics.Warn("assets", "Interrupted preparation recovered", "previous", previous, "outcome", "predecessor restored; readiness rechecked by caller")
	} else if err != nil {
		return err
	} else if record.Version == 2 {
		old := filepath.Join(filepath.Dir(target), record.Backup, "tree")
		if load == nil {
			return fmt.Errorf("pending pack promotion requires frontend validation")
		}
		resolved, e := filepath.EvalSymlinks(old)
		if e != nil || resolved != old {
			return fmt.Errorf("unidentified recovery predecessor at %s", old)
		}
		_, e = load(target)
		if e == nil {
			_, e = assetdata.LogicalResourceProfile(target)
		}
		if e != nil {
			// Preserve both trees, including evidence of the failed activation.
			rejected := filepath.Join(filepath.Dir(old), "rejected")
			if err = rename(target, rejected); err != nil {
				return err
			}
			if err = rename(old, target); err != nil {
				return err
			}
			fmt.Fprintf(log, "Restored previous assets after failed validation: %v\n", e)
		} else {
			if err = os.Remove(filename); err != nil {
				return err
			}
			if err = retireGenerated(old); err == nil {
				_ = os.Remove(filepath.Dir(old))
			} else {
				fmt.Fprintf(log, "Previous tree retained at %s: %v\n", old, err)
			}
			return nil
		}
	} else if err = ReplaceableAssets(target); err != nil {
		return err
	}
	return os.Remove(filename)
}

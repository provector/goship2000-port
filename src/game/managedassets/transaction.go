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
	Installations func() []Installation
	SourceUsed    func(string)
	Pack          func(string, string) error
	Prepare       func(string, string) (*assetdata.Manifest, error)
	Load          func(string) (*assetdata.OfficeAssets, error)
	Rename        func(string, string) error
	Ready         func(*assetdata.OfficeAssets) error
	Progress      func(assetdata.PreparationProgress)
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
			publishUsed(paths.Assets, s.SourceUsed)
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
	target, err := managedTarget(paths.Assets)
	if _, e := assetdata.ResolvedNewPath(paths.Original); e == nil && err == nil {
		_, target, err = ManagedPaths(paths.Original, paths.Assets)
	}
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
			publishUsed(target, s.SourceUsed)
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
	loose, used, fallbackReason := "", "", ""
	var fallbackSource Installation
	if manifest == nil {
		candidates := []Installation{{configuration.OriginalCustom, paths.Original}}
		if s.Installations != nil {
			var detected []Installation
			if paths.Config.OriginalSource != "" && paths.Config.OriginalSource != configuration.OriginalCustom {
				detected = s.Installations()
			}
			var selectionErr error
			candidates, selectionErr = sourceCandidates(paths.Config, paths.Directory, detected)
			if selectionErr != nil {
				_ = os.Remove(staging)
				return nil, selectionErr
			}
		}
		initialStaging := staging
		var failures []error
		for attempt, i := range candidates {
			if attempt != 0 {
				staging, err = os.MkdirTemp(filepath.Dir(target), "."+filepath.Base(target)+".preparing-")
				if err != nil {
					return nil, errors.Join(append(failures, err)...)
				}
				generated = filepath.Join(staging, "tree")
			}
			manifest, loose, err = prepareCandidate(i, target, staging, generated, log, s, load, emit)
			if err == nil {
				used = manifest.InputDirectory
				if used == "" {
					used = i.Path
				}
				if attempt != 0 {
					fallbackSource, fallbackReason = i, errors.Join(failures...).Error()
				}
				break
			}
			failures = append(failures, err)
			if !errors.Is(err, assetdata.ErrPreparationSource) || attempt == len(candidates)-1 {
				_ = os.Remove(initialStaging)
				return nil, errors.Join(failures...)
			}
		}
		if initialStaging != staging {
			_ = os.Remove(initialStaging)
		}
	} else {
		emit("Validate current prepared capabilities", 0, 1)
		if _, err = load(generated); err != nil {
			return nil, fmt.Errorf("generated office assets unusable at %s: %w; previous assets unchanged", staging, err)
		}
		used = manifest.InputDirectory
	}
	// The portable asset manifest deliberately omits the workstation source path.
	// Keep the actual source in the same staged generation as local provenance.
	if filepath.IsAbs(used) {
		b, _ := json.Marshal(used)
		if err = os.WriteFile(filepath.Join(generated, usedSourceFile), b, 0600); err != nil {
			return nil, fmt.Errorf("write local asset source: %w", err)
		}
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
	if fallbackReason != "" {
		fmt.Fprintf(log, "Using %s after source failure: %s\n", fallbackSource, fallbackReason)
		diagnostics.Info("assets", "Alternate original installation validated", "used", fallbackSource.String(), "reason", fallbackReason)
		diagnostics.StartupWarning("Prepared assets using "+fallbackSource.String()+" after the preferred installation failed.", fallbackReason)
	}
	if s.SourceUsed != nil && used != "" {
		s.SourceUsed(used)
	}
	diagnostics.Info("assets", "DOS SETUP preferences, campaigns and saved missions were skipped. GoShip settings and careers remain in the player data store.")
	emit("Prepared cache ready", 1, 1)
	return assets, nil
}

const usedSourceFile = "original-source.json"

func preparedSource(target string) (string, []byte) {
	path := filepath.Join(target, usedSourceFile)
	if st, e := os.Lstat(path); e != nil || !st.Mode().IsRegular() {
		return "", nil
	}
	b, e := assetdata.ReadLimitedFile(path, 32<<10)
	var source string
	if e == nil && json.Unmarshal(b, &source) == nil && filepath.IsAbs(source) {
		return source, b
	}
	return "", nil
}

func publishUsed(target string, use func(string)) {
	if use == nil {
		return
	}
	if source, _ := preparedSource(target); source != "" {
		diagnostics.Info("assets", "Prepared cache original source", "used", source)
		use(source)
	}
}

func prepareCandidate(i Installation, target, staging, generated string, log io.Writer, s Services, load func(string) (*assetdata.OfficeAssets, error), emit func(string, int, int)) (*assetdata.Manifest, string, error) {
	if st, e := os.Stat(i.Path); e != nil || !st.IsDir() {
		if e == nil {
			e = fmt.Errorf("expected a directory")
		}
		return nil, "", fmt.Errorf("%s: %w: %w: %w; select Original Installation in SETUP > PORT SETTINGS", i, assetdata.ErrPreparationSource, ErrOriginalUnavailable, e)
	}
	input, _, err := ManagedPaths(i.Path, target)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", i, err)
	}
	emit("Prepare assets from "+i.String(), 0, 0)
	output, loose := generated, ""
	if s.Pack != nil {
		loose = filepath.Join(staging, "conversion")
		output = loose
	}
	fmt.Fprintf(log, "Preparing assets from %s ...\n", i)
	diagnostics.Info("assets", "Selected installation for conversion", "original", input, "staging", output, "destination", target)
	manifest, err := s.Prepare(input, output)
	if manifest != nil {
		diagnostics.Info("assets", "Preparation manifest summary", "version", manifest.PreparationVersion, "result", manifest.Result, "inputs", len(manifest.Resources), "counts", manifest.Summary, "unsupported_operations", manifest.UnsupportedVersionOperations, "fatal", manifest.FatalError)
	}
	if err == nil && manifest == nil {
		err = fmt.Errorf("preparation returned no manifest")
	}
	if err == nil && s.Pack != nil {
		// Validate the fresh loose generation before packing. A missing prepared
		// capability reported along with decoding errors is a source rejection.
		var a *assetdata.OfficeAssets
		a, err = s.Load(loose)
		if err == nil && s.Ready != nil {
			err = s.Ready(a)
		}
		if err != nil && len(manifest.UnsupportedVersionOperations) > 0 && !errors.Is(err, os.ErrPermission) {
			err = fmt.Errorf("%w: %w", assetdata.ErrPreparationSource, err)
		}
		if err == nil {
			emit("Create and validate asset packs", 0, 1)
			err = s.Pack(loose, generated)
		}
	}
	if err == nil {
		emit("Validate current prepared capabilities", 0, 1)
		_, err = load(generated)
	}
	if err != nil {
		if manifest != nil && len(manifest.UnsupportedVersionOperations) > 0 {
			var details []string
			for _, op := range manifest.UnsupportedVersionOperations {
				details = append(details, op.Operation+": "+op.Reason)
			}
			err = fmt.Errorf("%w; preparation manifest: %s", err, strings.Join(details, "; "))
		}
		return nil, "", fmt.Errorf("preparing %s for %s (attempt retained at %s): %w", i, target, staging, err)
	}
	return manifest, loose, nil
}
func managedTarget(output string) (string, error) {
	out, err := assetdata.ResolvedNewPath(output)
	if err != nil {
		return "", err
	}
	if original, e := os.Lstat(output); e == nil && original.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to replace symlink asset directory %s", output)
	}
	return out, nil
}

func ManagedPaths(input, output string) (string, string, error) {
	in, err := assetdata.ResolvedNewPath(input)
	if err != nil {
		return "", "", err
	}
	out, err := managedTarget(output)
	if err != nil {
		return "", "", err
	}
	if assetdata.Within(in, out) || assetdata.Within(out, in) {
		return "", "", fmt.Errorf("original and assets directories must not overlap")
	}
	// Protect reference trees and original ancestors before creating output.
	if _, err = assetdata.GeneratedPath(filepath.Join(out, ".preparation-safety-check"), in); err != nil {
		return "", "", err
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

package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/menus"
	"gunship2000-assets/game/replay"
)

func runFilms(args []string) int {
	if len(args) > 0 && args[0] == "inspect-prefix" {
		flags := flag.NewFlagSet("inspect-prefix", flag.ContinueOnError)
		input := flags.String("input", "", "private G2R2 spool copy")
		if flags.Parse(args[1:]) != nil || *input == "" || flags.NArg() != 0 {
			return 1
		}
		blocks, ops, n, err := replay.InspectPrefix(*input)
		fmt.Printf("Verified prefix: %d frames, %d operations, %d bytes. Scan stopped at %v. No completion or score inferred.\n", blocks, ops, n, err)
		if blocks == 0 {
			return 1
		}
		return 0
	}
	if len(args) > 0 && args[0] == "verify" {
		return runFilmVerify(args[1:])
	}
	if len(args) > 0 && args[0] == "identify" {
		return runFilmIdentify(args[1:])
	}
	if len(args) == 1 && (args[0] == "-help" || args[0] == "--help") {
		fmt.Fprintln(os.Stdout, "usage: gs2000 films identify -input FILE.g2r\n       gs2000 films verify -input COPY.g2r -assets PREPARED\n       gs2000 films recover -input COPY.g2r -receipt NEW.json -assets PREPARED\n       gs2000 films verify-recovery -input COPY.g2r -receipt EXISTING.json -assets PREPARED")
		return 0
	}
	if len(args) == 0 || (args[0] != "recover" && args[0] != "verify-recovery") {
		fmt.Fprintln(os.Stderr, "usage: gs2000 films identify -input FILE.g2r\n       gs2000 films verify -input COPY.g2r -assets PREPARED\n       gs2000 films recover -input COPY.g2r -receipt NEW.json -assets PREPARED\n       gs2000 films verify-recovery -input COPY.g2r -receipt EXISTING.json -assets PREPARED")
		return 1
	}
	mode := args[0]
	flags := flag.NewFlagSet("gs2000 films "+mode, flag.ContinueOnError)
	input := flags.String("input", "", "explicit native film (read only; use a preserved copy)")
	receipt := flags.String("receipt", "", "explicit recovery receipt; recover creates a NEW file")
	directory := flags.String("assets", "", "complete prepared assets; no original/research dependency")
	if e := flags.Parse(args[1:]); e != nil {
		if e == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 || *input == "" || *receipt == "" || *directory == "" {
		fmt.Fprintln(os.Stderr, "films: supply -input COPY.g2r -receipt FILE.json -assets PREPARED")
		return 1
	}
	in, e := filepath.Abs(*input)
	if e != nil {
		return startupError(e)
	}
	out, e := filepath.Abs(*receipt)
	if e != nil {
		return startupError(e)
	}
	if in == out {
		return startupError(fmt.Errorf("receipt cannot replace the film"))
	}
	info, e := os.Lstat(in)
	if e != nil {
		return startupError(e)
	}
	if !info.Mode().IsRegular() || info.Size() > replay.MaxBytes {
		return startupError(fmt.Errorf("input must be a bounded regular film file"))
	}
	if mode == "recover" {
		owner, err := configuration.OwnOutput(out)
		if err != nil {
			return startupError(err)
		}
		defer owner.Close()
		if _, e = os.Lstat(out); e == nil {
			return startupError(fmt.Errorf("receipt output already exists"))
		} else if !os.IsNotExist(e) {
			return startupError(e)
		}
	}
	f, e := replay.OpenFile(in)
	if e != nil {
		return startupError(e)
	}
	digest, e := replay.RecoveryDigest(in)
	if e != nil {
		return startupError(e)
	}
	a, e := assets.LoadFrontEndAssets(*directory)
	if e != nil {
		return startupError(e)
	}
	if mode == "recover" {
		r, e := menus.RecoverLegacySource(a, f, digest, configuration.Defaults())
		if e != nil {
			return startupError(e)
		}
		// The source is read-only, but another process may have edited it while
		// the bounded search ran. Do not publish a receipt for stale bytes.
		current, err := replay.RecoveryDigest(in)
		if err != nil {
			return startupError(err)
		}
		if current != r.FilmSHA256 {
			return startupError(fmt.Errorf("film changed during recovery; no receipt published"))
		}
		if e = replay.WriteRecoveryNew(out, r); e != nil {
			return startupError(e)
		}
		fmt.Printf("Verified recovery: film %s, operation%d, overview cursor%d/%d; %d logical steps and %d events.\nOriginal film SHA256: %s\nReceipt: %s\n", r.FilmID, r.Operation, r.CursorX, r.CursorY, r.Steps, r.Events, r.SourceSHA256, out)
		return 0
	}
	r, e := replay.ReadRecovery(out)
	if e != nil {
		return startupError(e)
	}
	if _, e = menus.NewRecoveredSource(a, f, digest, r, configuration.Defaults()); e != nil {
		return startupError(e)
	}
	fmt.Printf("Recovery and complete original stream verified; film %s remains unchanged.\n", r.FilmID)
	return 0
}

// identify validates bounded container/profile framing only. It prints exactly
// the stored identity for safe managed-copy naming, never constructs a session
// or touches preferences, assets, receipts or careers.
func runFilmIdentify(args []string) int {
	flags := flag.NewFlagSet("gs2000 films identify", flag.ContinueOnError)
	input := flags.String("input", "", "explicit native film (read only; filename need not be its ID)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if flags.NArg() != 0 || *input == "" {
		fmt.Fprintln(os.Stderr, "films identify: supply -input FILE.g2r")
		return 1
	}
	film, err := replay.OpenFile(*input)
	if err != nil {
		return startupError(err)
	}
	fmt.Fprintln(os.Stdout, film.Header.ID)
	return 0
}

// runFilmVerify constructs the capability-isolated ordinary reader. It neither
// searches recovery values nor opens saves, preferences or a result publisher.
func runFilmVerify(args []string) int {
	flags := flag.NewFlagSet("gs2000 films verify", flag.ContinueOnError)
	input := flags.String("input", "", "bounded native film (read only)")
	directory := flags.String("assets", "", "verified prepared resources")
	if e := flags.Parse(args); e != nil {
		if e == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if *input == "" || *directory == "" || flags.NArg() != 0 {
		return startupError(fmt.Errorf("films verify requires -input and -assets"))
	}
	st, e := os.Lstat(*input)
	if e != nil {
		return startupError(e)
	}
	if !st.Mode().IsRegular() || st.Size() > replay.MaxBytes {
		return startupError(fmt.Errorf("bounded regular film required"))
	}
	f, e := replay.OpenFile(*input)
	if e != nil {
		return startupError(e)
	}
	a, e := assets.LoadFrontEndAssets(*directory)
	if e != nil {
		return startupError(e)
	}
	q, e := menus.NewFilmPlayback(a, f, configuration.Defaults())
	if e != nil {
		return startupError(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			return startupError(e)
		}
	}
	if q.Engine.Store != nil {
		return startupError(fmt.Errorf("film acquired career capability"))
	}
	in, e := os.Open(*input)
	if e != nil {
		return startupError(e)
	}
	defer in.Close()
	h := sha256.New()
	if _, e = io.Copy(h, io.LimitReader(in, replay.MaxBytes+1)); e != nil {
		return startupError(e)
	}
	fmt.Printf("Verified film%s SHA256%x: %d operations, %d logical steps, %d events, score%d; all checkpoint groups and ending match.\n", f.Header.ID, h.Sum(nil), f.OperationCount(), f.End.Steps, q.EventCount, f.End.Score)
	return 0
}

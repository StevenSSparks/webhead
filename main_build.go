package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/stevenssparks/roost/image"
)

// ---------------------------------------------------------------------------
// roost doctor — check and install the hardware toolchain
// ---------------------------------------------------------------------------

func cmdDoctor(args []string) {
	fset := flag.NewFlagSet("doctor", flag.ExitOnError)
	install := fset.Bool("install", false, "install missing tools (Homebrew + esp32 core)")
	fset.Parse(args)

	fmt.Println("roost doctor — hardware toolchain")
	missing := []string{}
	check := func(name string, bins ...string) {
		for _, b := range bins {
			if p, err := exec.LookPath(b); err == nil {
				fmt.Printf("  ok   %-12s %s\n", name, p)
				return
			}
		}
		fmt.Printf("  MISS %-12s not found\n", name)
		missing = append(missing, name)
	}
	check("arduino-cli", "arduino-cli")
	check("esptool", "esptool.py", "esptool")
	check("mklittlefs", "mklittlefs")

	core := esp32Core()
	if core != "" {
		fmt.Printf("  ok   %-12s %s\n", "esp32-core", core)
	} else {
		fmt.Printf("  MISS %-12s not installed\n", "esp32-core")
		missing = append(missing, "esp32-core")
	}

	if len(missing) == 0 {
		fmt.Println("\nAll set — `roost build-image` and `roost flash-board` are ready.")
		return
	}
	if !*install {
		fmt.Printf("\nMissing: %s\nInstall them with:  roost doctor --install\n", strings.Join(missing, ", "))
		return
	}

	if _, err := exec.LookPath("brew"); err != nil {
		fatal(fmt.Errorf("Homebrew not found — install it from https://brew.sh, then re-run `roost doctor --install`"))
	}
	fmt.Println("\n==> brew install arduino-cli esptool mklittlefs")
	_ = run("brew", "install", "arduino-cli", "esptool", "mklittlefs")
	if esp32Core() == "" {
		fmt.Println("==> installing the esp32 Arduino core")
		_ = run("arduino-cli", "config", "init")
		_ = run("arduino-cli", "config", "add", "board_manager.additional_urls",
			"https://raw.githubusercontent.com/espressif/arduino-esp32/gh-pages/package_esp32_index.json")
		_ = run("arduino-cli", "core", "update-index")
		_ = run("arduino-cli", "core", "install", "esp32:esp32")
	}
	fmt.Println("\n==> done. Re-run `roost doctor` to verify.")
}

func esp32Core() string {
	out, err := exec.Command("arduino-cli", "core", "list").Output()
	if err != nil {
		return ""
	}
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(ln, "esp32:esp32") {
			if f := strings.Fields(ln); len(f) >= 2 {
				return f[1]
			}
			return "installed"
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// roost build-image — image folder -> one flashable .bin
// ---------------------------------------------------------------------------

func cmdBuildImage(args []string) {
	fset := flag.NewFlagSet("build-image", flag.ExitOnError)
	sketch := fset.String("sketch", "", "firmware sketch dir (default: <image>/firmware, else the built-in reference)")
	fqbn := fset.String("fqbn", "esp32:esp32:XIAO_ESP32S3", "board FQBN")
	chip := fset.String("chip", "esp32s3", "target chip")
	partition := fset.String("partition", "", "esp32 partition scheme (e.g. large_spiffs_8MB); auto-sets FS offset/size")
	offset := fset.String("offset", "0x670000", "FS offset (default_8MB scheme)")
	size := fset.String("size", "0x180000", "FS size (default_8MB scheme)")
	bootOff := fset.String("boot-offset", "0x0", "bootloader offset (esp32s3=0x0, classic esp32=0x1000)")
	out := fset.String("out", "", "output image path (default <image>/build/<name>-8MB.bin)")
	imgArg, rest := splitImageArgs(args)
	fset.Parse(rest)
	if imgArg == "" {
		imgArg = fset.Arg(0)
	}
	if imgArg == "" {
		fatal(fmt.Errorf("build-image needs an image path (the dir holding roost.json + data/)"))
	}

	im, err := image.Load(imgArg)
	if err != nil {
		fatal(err)
	}
	dataDir := filepath.Join(im.Dir, "data")

	acli, e1 := exec.LookPath("arduino-cli")
	mk, e2 := exec.LookPath("mklittlefs")
	esp, e3 := findEsptool()
	if e1 != nil || e2 != nil || e3 != nil {
		fatal(fmt.Errorf("missing hardware tools — run:  roost doctor --install"))
	}

	if *partition != "" {
		if o, s, err := lookupPartitionFS(*partition); err == nil {
			*offset, *size = o, s
			fmt.Printf("partition : %s -> offset %s size %s\n", *partition, o, s)
		} else {
			fmt.Printf("[warn] %v — using --offset/--size\n", err)
		}
	}

	// fit check
	if pb, err := parseHexSize(*size); err == nil {
		used := dirSize(dataDir)
		if used > pb {
			fatal(fmt.Errorf("payload %d KB exceeds the %d KB filesystem — use --partition large_spiffs_8MB", used/1024, pb/1024))
		}
		fmt.Printf("payload   : %d KB of %d KB FS (%d%%)\n", used/1024, pb/1024, int(used*100/pb))
	}

	work, name, cleanup, err := prepareSketch(*sketch, im)
	if err != nil {
		fatal(err)
	}
	defer cleanup()

	fwout := filepath.Join(os.TempDir(), "roost-build-fw")
	os.RemoveAll(fwout)
	cargs := []string{"compile", "--fqbn", *fqbn, "--output-dir", fwout}
	if *partition != "" {
		cargs = append(cargs, "--build-property", "build.partitions="+*partition)
	}
	cargs = append(cargs, work)
	fmt.Printf("==> compiling firmware: %s (%s)\n", name, *fqbn)
	if err := run(acli, cargs...); err != nil {
		fatal(fmt.Errorf("firmware compile failed: %w", err))
	}

	boot := firstGlob(fwout, "*.bootloader.bin")
	parts := firstGlob(fwout, "*.partitions.bin")
	app := firstGlob(fwout, "*.ino.bin")
	bootapp0 := coreBootApp0()
	if boot == "" || parts == "" || app == "" || bootapp0 == "" {
		fatal(fmt.Errorf("could not locate build outputs (boot=%q parts=%q app=%q boot_app0=%q)", boot, parts, app, bootapp0))
	}

	lfs := filepath.Join(os.TempDir(), "roost-build-littlefs.bin")
	fmt.Println("==> building LittleFS from data/")
	if err := run(mk, "-c", dataDir, "-p", "256", "-b", "4096", "-s", *size, lfs); err != nil {
		fatal(fmt.Errorf("mklittlefs failed: %w", err))
	}

	if *out == "" {
		*out = filepath.Join(im.Dir, "build", slug(im.Manifest.Name)+"-8MB.bin")
	}
	os.MkdirAll(filepath.Dir(*out), 0755)
	fmt.Println("==> merging one flashable image")
	if err := run(esp, "--chip", *chip, "merge-bin", "--flash-size", "8MB", "-o", *out,
		*bootOff, boot, "0x8000", parts, "0xe000", bootapp0, "0x10000", app, *offset, lfs); err != nil {
		fatal(fmt.Errorf("esptool merge-bin failed: %w", err))
	}

	fmt.Printf("\nbuilt: %s\n", *out)
	fmt.Printf("flash it:  roost flash-board %s --image %s --confirm\n", imgArg, *out)
	fmt.Printf("      or:  esptool --chip %s -p <PORT> write_flash 0x0 %s\n", *chip, *out)
}

// prepareSketch returns a temp working copy of the firmware sketch (named after
// its .ino so arduino-cli accepts it) with a generated roost_config.h carrying the
// image's SSID/domain. Source: --sketch, else <image>/firmware, else the
// embedded reference firmware.
func prepareSketch(sketchFlag string, im *image.Image) (workDir, name string, cleanup func(), err error) {
	cleanup = func() {}
	var srcFiles map[string][]byte

	readDisk := func(dir string) (map[string][]byte, string, error) {
		var ino string
		files := map[string][]byte{}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, "", err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, "", err
			}
			files[e.Name()] = b
			if strings.HasSuffix(e.Name(), ".ino") {
				ino = e.Name()
			}
		}
		if ino == "" {
			return nil, "", fmt.Errorf("no .ino in %s", dir)
		}
		return files, ino, nil
	}

	var ino string
	switch {
	case sketchFlag != "":
		srcFiles, ino, err = readDisk(sketchFlag)
	case dirHasIno(filepath.Join(im.Dir, "firmware")):
		srcFiles, ino, err = readDisk(filepath.Join(im.Dir, "firmware"))
	default:
		srcFiles, ino, err = readEmbeddedFirmware()
	}
	if err != nil {
		return "", "", cleanup, err
	}

	base := strings.TrimSuffix(ino, ".ino")
	root, err := os.MkdirTemp("", "roost-sketch-")
	if err != nil {
		return "", "", cleanup, err
	}
	cleanup = func() { os.RemoveAll(root) }
	dir := filepath.Join(root, base)
	os.MkdirAll(dir, 0755)
	for n, b := range srcFiles {
		os.WriteFile(filepath.Join(dir, n), b, 0644)
	}
	// generate roost_config.h from the manifest identity
	cfg := fmt.Sprintf("// generated by `roost build-image`\n#define RO_SSID %q\n#define RO_DOMAIN %q\n",
		im.Manifest.SSID, im.Manifest.Domain)
	os.WriteFile(filepath.Join(dir, "roost_config.h"), []byte(cfg), 0644)
	return dir, base, cleanup, nil
}

func readEmbeddedFirmware() (map[string][]byte, string, error) {
	files := map[string][]byte{}
	var ino string
	err := fs.WalkDir(referenceFirmware, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, e := referenceFirmware.ReadFile(p)
		if e != nil {
			return e
		}
		files[filepath.Base(p)] = b
		if strings.HasSuffix(p, ".ino") {
			ino = filepath.Base(p)
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if ino == "" {
		return nil, "", fmt.Errorf("embedded reference firmware missing .ino")
	}
	return files, ino, nil
}

func dirHasIno(dir string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, "*.ino"))
	return len(m) > 0
}

func firstGlob(dir, pat string) string {
	m, _ := filepath.Glob(filepath.Join(dir, pat))
	if len(m) > 0 {
		return m[0]
	}
	return ""
}

func coreBootApp0() string {
	home, _ := os.UserHomeDir()
	for _, base := range []string{"Library/Arduino15", ".arduino15"} {
		m, _ := filepath.Glob(filepath.Join(home, base, "packages/esp32/hardware/esp32/*/tools/partitions/boot_app0.bin"))
		if len(m) > 0 {
			return m[0]
		}
	}
	return ""
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "image"
	}
	return out
}

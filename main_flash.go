package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/stevenssparks/webhead/image"
)

// cmdFlashBoard builds a LittleFS image from an image's data/ payload and flashes
// it to a connected ESP32 (e.g. the Seeed XIAO ESP32-S3) — so what you tested in
// the emulator is what ships on the board. It PLANS by default; pass --confirm to
// actually write to hardware.
func cmdFlashBoard(args []string) {
	fs := flag.NewFlagSet("flash-board", flag.ExitOnError)
	port := fs.String("port", "", "serial port (auto-detected if empty)")
	chip := fs.String("chip", "esp32s3", "target chip")
	offset := fs.String("offset", "0x670000", "LittleFS partition offset (default_8MB scheme)")
	size := fs.String("size", "0x180000", "LittleFS partition size (default_8MB scheme)")
	partition := fs.String("partition", "", "esp32 partition scheme name (e.g. large_spiffs_8MB) — auto-sets offset/size from the installed core")
	page := fs.Int("page", 256, "LittleFS page size")
	block := fs.Int("block", 4096, "LittleFS block size")
	confirm := fs.Bool("confirm", false, "actually write to the board (default: plan only)")
	fullImage := fs.String("image", "", "flash a prebuilt full image (from `build-image`) at 0x0 instead of just the FS")
	imgArg, rest := splitImageArgs(args)
	fs.Parse(rest)
	if imgArg == "" {
		imgArg = fs.Arg(0)
	}

	// --image: write a complete prebuilt image (firmware+FS) at 0x0.
	if *fullImage != "" {
		flashFullImage(*fullImage, *chip, *port, *confirm)
		return
	}

	if imgArg == "" {
		fatal(fmt.Errorf("flash-board needs an image path (the dir holding data/)"))
	}

	im, err := image.Load(imgArg)
	if err != nil {
		fatal(err)
	}
	dataDir := filepath.Join(im.Dir, "data")

	// Let webhead figure out the offset/size from the named partition scheme so
	// nobody has to hand-enter hex.
	if *partition != "" {
		if off, sz, err := lookupPartitionFS(*partition); err == nil {
			*offset, *size = off, sz
			fmt.Printf("    partition: %s → offset %s, size %s (from esp32 core)\n", *partition, off, sz)
		} else {
			fmt.Printf("    [warn] partition %q: %v — falling back to --offset/--size\n", *partition, err)
		}
	}

	// Does the payload fit the FS partition?
	if partBytes, err := parseHexSize(*size); err == nil {
		used := dirSize(dataDir)
		pct := 0
		if partBytes > 0 {
			pct = int(used * 100 / partBytes)
		}
		fmt.Printf("    payload  : %d KB of %d KB partition (%d%%)\n", used/1024, partBytes/1024, pct)
		if used > partBytes {
			fmt.Printf("    ⚠ payload is LARGER than the FS partition. Use a bigger scheme, e.g.\n")
			fmt.Printf("      webhead flash-board %s --partition large_spiffs_8MB\n", imgArg)
		}
	}

	mklittlefs, mkErr := exec.LookPath("mklittlefs")
	esptool, espErr := findEsptool()

	fmt.Printf("==> flash-board: %s\n", im.Manifest.Name)
	fmt.Printf("    payload : %s\n", dataDir)
	fmt.Printf("    chip    : %s   offset %s   size %s   (page %d, block %d)\n", *chip, *offset, *size, *page, *block)

	// Auto-detect the port if not given.
	if *port == "" {
		*port = detectPort()
	}
	if *port == "" {
		fmt.Println("    port    : (none detected — plug in the board or pass --port)")
	} else {
		fmt.Printf("    port    : %s\n", *port)
	}

	// Report tool availability.
	missing := false
	if mkErr != nil {
		fmt.Println("    ⚠ mklittlefs not found — install: https://github.com/earlephilhower/mklittlefs")
		missing = true
	}
	if espErr != nil {
		fmt.Println("    ⚠ esptool not found — install: pip install esptool")
		missing = true
	}

	tmp := filepath.Join(os.TempDir(), "webhead-littlefs.bin")
	buildCmd := fmt.Sprintf("%s -c %s -p %d -b %d -s %s %s", orName(mklittlefs, "mklittlefs"), dataDir, *page, *block, *size, tmp)
	flashCmd := fmt.Sprintf("%s --chip %s --port %s write_flash %s %s", orName(esptool, "esptool"), *chip, orName(*port, "<port>"), *offset, tmp)

	fmt.Println("\n    plan:")
	fmt.Printf("      1) %s\n", buildCmd)
	fmt.Printf("      2) %s\n", flashCmd)
	fmt.Println("\n    NOTE: the offset/size MUST match your board's partition scheme")
	fmt.Println("          (Arduino IDE → Tools → Partition Scheme). Wrong values can")
	fmt.Println("          overwrite the firmware. Verify before using --confirm.")

	if !*confirm {
		fmt.Println("\n==> plan only. Re-run with --confirm to write to the board.")
		return
	}
	if missing || *port == "" {
		fatal(fmt.Errorf("cannot flash: missing tools or no port (see warnings above)"))
	}

	fmt.Println("\n==> building LittleFS image")
	if err := run(mklittlefs, "-c", dataDir, "-p", fmt.Sprint(*page), "-b", fmt.Sprint(*block), "-s", *size, tmp); err != nil {
		fatal(fmt.Errorf("mklittlefs failed: %w", err))
	}
	fmt.Println("==> writing to board")
	if err := run(esptool, "--chip", *chip, "--port", *port, "write_flash", *offset, tmp); err != nil {
		fatal(fmt.Errorf("esptool failed: %w", err))
	}
	fmt.Println("==> done. Reboot the board; it now serves this image's data/ over LittleFS.")
}

// flashFullImage writes a complete prebuilt image (bootloader+partitions+app+FS)
// at offset 0x0.
func flashFullImage(path, chip, port string, confirm bool) {
	if _, err := os.Stat(path); err != nil {
		fatal(fmt.Errorf("image not found: %s (build it with `webhead build-image`)", path))
	}
	esptool, err := findEsptool()
	if err != nil {
		fatal(fmt.Errorf("esptool not found — run: webhead doctor --install"))
	}
	if port == "" {
		port = detectPort()
	}
	fmt.Printf("==> full-image flash\n    image : %s\n    chip  : %s\n    port  : %s\n",
		path, chip, orName(port, "(none detected — plug in the board or pass --port)"))
	fmt.Printf("\n    plan:\n      %s --chip %s --port %s write_flash 0x0 %s\n", esptool, chip, orName(port, "<port>"), path)
	if !confirm {
		fmt.Println("\n==> plan only. Re-run with --confirm to write to the board.")
		return
	}
	if port == "" {
		fatal(fmt.Errorf("no serial port; plug in the board or pass --port"))
	}
	fmt.Println("\n==> writing (erases + writes; ~30-60s)")
	if err := run(esptool, "--chip", chip, "--port", port, "write_flash", "0x0", path); err != nil {
		fatal(fmt.Errorf("esptool failed: %w", err))
	}
	fmt.Println("==> done. Reset the board.")
}

// findEsptool looks for esptool.py or esptool on PATH.
func findEsptool() (string, error) {
	for _, name := range []string{"esptool.py", "esptool"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("esptool not found")
}

// detectPort returns a likely ESP32 serial port, or "" if none is obvious.
func detectPort() string {
	entries, err := filepath.Glob("/dev/cu.usb*")
	if err != nil || len(entries) == 0 {
		entries, _ = filepath.Glob("/dev/ttyUSB*")
	}
	if len(entries) > 0 {
		return entries[0]
	}
	return ""
}

func orName(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// lookupPartitionFS finds the LittleFS/SPIFFS partition offset+size for a named
// esp32 partition scheme (e.g. "large_spiffs_8MB") from the installed core's
// partition CSVs, so users don't hand-enter hex.
func lookupPartitionFS(name string) (offset, size string, err error) {
	home, _ := os.UserHomeDir()
	glob := filepath.Join(home, "Library/Arduino15/packages/esp32/hardware/esp32/*/tools/partitions", name+".csv")
	matches, _ := filepath.Glob(glob)
	if len(matches) == 0 {
		// also try Linux Arduino data dir
		matches, _ = filepath.Glob(filepath.Join(home, ".arduino15/packages/esp32/hardware/esp32/*/tools/partitions", name+".csv"))
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("partition CSV %q not found (is the esp32 core installed?)", name)
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		return "", "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, ",")
		if len(f) >= 5 {
			sub := strings.TrimSpace(f[2])
			if sub == "spiffs" || sub == "littlefs" {
				return strings.TrimSpace(f[3]), strings.TrimSpace(f[4]), nil
			}
		}
	}
	return "", "", fmt.Errorf("no spiffs/littlefs partition in %s", name)
}

func parseHexSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	return strconv.ParseInt(s, 16, 64)
}

func dirSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	offset := fs.String("offset", "0x670000", "LittleFS partition offset (depends on your partition scheme!)")
	size := fs.String("size", "0x160000", "LittleFS partition size")
	page := fs.Int("page", 256, "LittleFS page size")
	block := fs.Int("block", 4096, "LittleFS block size")
	confirm := fs.Bool("confirm", false, "actually write to the board (default: plan only)")
	imgArg, rest := splitImageArgs(args)
	fs.Parse(rest)
	if imgArg == "" {
		imgArg = fs.Arg(0)
	}
	if imgArg == "" {
		fatal(fmt.Errorf("flash-board needs an image path (the dir holding data/)"))
	}

	im, err := image.Load(imgArg)
	if err != nil {
		fatal(err)
	}
	dataDir := filepath.Join(im.Dir, "data")

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

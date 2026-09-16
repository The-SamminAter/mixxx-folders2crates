package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/hashicorp/go-multierror"
	ignore "github.com/sabhiram/go-gitignore"

	"github.com/bvobart/mixxx-folders2crates/folders2crates"
	"github.com/bvobart/mixxx-folders2crates/mixxxdb"
	"github.com/bvobart/mixxx-folders2crates/utils"
)

var bold = color.New(color.Bold)
var faint = color.New(color.Faint)
var green = color.New(color.FgGreen)
var red = color.New(color.FgRed)
var yellow = color.New(color.FgYellow)

func main() {
	if mixxxdb.DefaultMixxxDBPath == "" {
		red.Println("Error: your OS is unsupported, no known path to Mixxx's DB on", runtime.GOOS) //TODO: custom path
		os.Exit(1)
	}

	startTime := time.Now()
	libfolder, flat, crateName := parseArgs(os.Args)

	green.Println("Mixxx DB:     ", color.HiWhiteString(mixxxdb.DefaultMixxxDBPath))
	green.Println("Music Library:", color.HiWhiteString(libfolder))
	fmt.Println()

	// parse .crateignore
	ignoreFile, err := ignore.CompileIgnoreFile(path.Join(libfolder, ".crateignore"))
	if err != nil {
		ignoreFile = &ignore.GitIgnore{}
	}

	// detect which folders in the music library should become crates and what tracks should be in them according to the folder layout.
	crates, err := folders2crates.FindCrateFolders(libfolder, ignoreFile, flat, crateName)
	if err != nil {
		red.Println("Error detecting crates from your music library:")
		red.Println("  ", yellow.Sprint(err.Error()))
		os.Exit(5)
	}

	if len(crates) == 0 {
		green.Println("No crates found, nothing to do!")
		return
	}

	// open Mixxx's database
	db, err := mixxxdb.OpenDefault()
	if err != nil {
		red.Println("Error opening Mixxx's DB:")
		red.Println("  ", yellow.Sprint(err.Error()))
		os.Exit(1)
	}

	// temporary: print all crates
	for _, crate := range crates {
		if len(crate.Tracks) == 0 {
			continue
		}

		bold.Print(crate.Name, " (", len(crate.Tracks), " tracks)")
		dbCrate, err := db.Crates().FindByName(crate.Name)
		if err != nil {
			panic(err)
		}

		if dbCrate != nil {
			green.Println(" - exists!")
		} else {
			fmt.Println()
		}

		for i, track := range crate.Tracks {
			if i < len(crate.Tracks)-1 {
				fmt.Print("├── ")
			} else {
				fmt.Print("└── ")
			}
			faint.Print(".", strings.TrimPrefix(path.Dir(string(track)), libfolder), "/")
			fmt.Println(path.Base(string(track)))
		}
		fmt.Println()
	}

	// if used in a terminal instead of in a script, ask confirmation
	if utils.IsInteractive() {
		pauseTime := time.Now()

		nCrates := len(crates)
		nTracks := folders2crates.CountTracks(crates)
		if err := utils.PromptConfirm("Are you sure you want these %d crates, totalling %d tracks, in your Mixxx DB?", nCrates, nTracks); err != nil {
			fmt.Println()
			yellow.Println("Alright, no problem, just let me know when you need those crates inserted! 😊")
			return
		}

		startTime = startTime.Add(time.Since(pauseTime)) // ignores the time taken to confirm
		fmt.Println()
	}

	yellow.Println("Inserting ", bold.Sprint(len(crates)), yellow.Sprint(" crates into Mixxx's DB..."))
	fmt.Println()

	// update the crates in Mixxx' DB
	var multierr *multierror.Error
	for _, crate := range crates {
		t := time.Now()
		err := folders2crates.UpdateCrateInDB(db, crate)
		multierr = multierror.Append(multierr, err)
		if err == nil {
			green.Print("✅ ", crate.Name, strings.Repeat(" ", int(math.Max(0, float64(48-len(crate.Name))))))
		} else {
			red.Print("❌ ", crate.Name, strings.Repeat(" ", int(math.Max(0, float64(48-len(crate.Name))))))
		}
		faint.Println("\t", time.Since(t))
	}

	fmt.Println()

	err = multierr.ErrorOrNil()
	if errors.Is(err, folders2crates.ErrTrackNotFound) {
		yellow.Println("Warning!")
		yellow.Println("  There were one or more tracks that couldn't be found in Mixxx's library.")
		yellow.Println("  The easiest way to fix this problem is to start Mixxx, let it re-scan your library, then close Mixxx and run this program again.")
		fmt.Println()
	}
	if err != nil {
		red.Println("Error:")
		red.Println("  ", yellow.Sprint(err.Error()))
		fmt.Println()
		faint.Println("took:", time.Since(startTime))
		os.Exit(1)
	}

	green.Println("Done!")
	faint.Println("took:", time.Since(startTime))
}

// parseArgs parses the arguments passed to folders2crates, deals with invalid arguments and returns the one valid argument: the path to a music library folder
func parseArgs(args []string) (string, bool, string) {
	if len(args) < 2 || args[1] == "-h" || args[1] == "--help" {
		red.Println("Expecting a music library folder as argument, but nothing was provided")
		red.Println("")
		green.Println("Use: ", args[0], "<dir> <args> \"<optional name>\"")
		green.Println("-f/--flat		Only create a single crate containing tracks")
		green.Println("-n/--name <name>	Names the new crate $name instead of the directory name")
		green.Println("		Mixxx does not support sub-crates, so use of -n must be used alongside -f")
		red.Println("")
		red.Println("Version: 1.0-mod-The_SamminAter") //WARNING: not sure where printVersion() gets version from, doesn't seem to work? compile error
		os.Exit(1)
	}

	if !utils.FileExists(mixxxdb.DefaultMixxxDBPath) {
		red.Println("Cannot open your Mixxx DB, because it is not present at", mixxxdb.DefaultMixxxDBPath)
		yellow.Println("Try to start Mixxx, close it, then run this program again")
		os.Exit(1)
	}

	libfolder := args[1]
	if !utils.FolderExists(libfolder) {
		if utils.FileExists(libfolder) {
			red.Println("Error: expected a directory but got a file")
			os.Exit(1)
		}
		red.Println("Error: directory", libfolder, "does not exist in the current path")
		os.Exit(1)
	}

	flat := false
	crateName := ""
	if len(args) > 2 {
		if args[2] == "-f" || args[2] == "--flat" {
			flat = true
			if len(args) > 3 {
				if args[3] != "-n" && args[3] != "--name" {
					red.Println("Warning: unexpected string encountered at $3")
				} else { //So, if $3 is -n/--name
					if len(args) > 4 {
						crateName = args[4]
						if len(args) > 5 {
							red.Println("Warning: unexpected string encountered at $5 (and possibly after)")
						}
					}
				}
			}
		} else if args[2] == "-n" || args[2] == "--name" {
			crateName = args[3]
			if len(args) > 4 {
				if args[4] == "-f" || args[4] == "--flat" {
					flat = true
				} else {
					red.Println("Warning: unexpected string encountered at $4 (and possibly after)")
				}
				if len(args) > 5 {
					red.Println("Warning: unexpected string encountered at $5 (and possibly after)")
				}
			} else {
				red.Println("Error: mixxx does not support sub-crates, so to use this flag add -f to enable flat mode")
				os.Exit(1)
			}
		}
	}

	libfolder, _ = filepath.Abs(libfolder)
	return libfolder, flat, crateName
}

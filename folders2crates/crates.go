package folders2crates

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

type CrateFolder struct {
	Name   string
	Tracks []TrackFile
}

// Counts the total number of tracks in the given array of CrateFolders.
func CountTracks(crates []CrateFolder) int {
	total := 0
	for _, crate := range crates {
		total += len(crate.Tracks)
	}
	return total
}

// FindCrateFolders searches the given folder for music tracks that Mixxx can play.
// When it finds a folder with at least track in it, it will make a crate with the name of the path to that folder
// and the tracks that are directly inside the folder.
// Respects the ignore patterns specified with the github.com/sabhiram/go-gitignore library.
// Note: Tracks will not have any database IDs.
func FindCrateFolders(libfolder string, ignore *ignore.GitIgnore, flat bool, crateName string) ([]CrateFolder, error) {
	crates := []CrateFolder{}
	paths := strings.Split(libfolder, "/")
	toppath := paths[len(paths)-1]

	err := filepath.WalkDir(libfolder, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// ignore the library root directory and anything that's not a directory.
		if !d.IsDir() || fpath == libfolder {
			return nil
		}

		// ignore folders from .crateignore
		if ignore != nil && ignore.MatchesPath(fpath) {
			return nil
		}

		// when encountering a non-ignored directory, find all tracks in it and make a crate
		relpath := strings.TrimPrefix(fpath, libfolder+"/")
		tracks, err := FindTrackFiles(fpath, ignore)
		if err != nil {
			return fmt.Errorf("failed to find tracks in %s: %w", relpath, err)
		}

		// skip folders that don't contain any tracks.
		if len(tracks) == 0 {
			return nil
		}

		name := NameCrate(relpath) //This just serves to make ONE crate, by overriding the local name... but it doesn't work
		if flat {
			if crateName != "" {
				name = NameCrate(crateName)
			} else {
				name = NameCrate(toppath)
			}
		}

		if flat {
			if len(crates) < 1 {
				crates = append(crates, CrateFolder{Name: name, Tracks: tracks})
			} else {
				crates[0].Tracks = append(crates[0].Tracks, tracks...)
				//Apparently ... means append each element of the array (tracks)
				//This was very hard to find/figure out, as someone who doesn't know go
			}
		} else {
			crates = append(crates, CrateFolder{Name: name, Tracks: tracks})
		}
		return nil
	})

	return crates, err
}

// NameCrate creates a name for a crate based on its path relative to the music library.
// Replaces folder separators with -
// E.g. House/90's becomes House - 90's
func NameCrate(relpath string) string {
	return fmt.Sprint(strings.Replace(relpath, string(os.PathSeparator), " - ", -1))
}

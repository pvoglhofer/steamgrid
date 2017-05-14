// Automatically downloads and configures Steam grid images for all games in a
// given Steam installation.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Prints an error and quits.
func errorAndExit(err error) {
	fmt.Println(err.Error())
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	os.Exit(0)
}

func main() {
	http.DefaultTransport.(*http.Transport).ResponseHeaderTimeout = time.Second * 10
	startApplication()
}

func startApplication() {
	fmt.Println("Loading overlays...")
	overlays, err := LoadOverlays(filepath.Join(filepath.Dir(os.Args[0]), "overlays by category"))
	if err != nil {
		errorAndExit(err)
	}
	if len(overlays) == 0 {
		fmt.Println("No category overlays found. You can put overlay images in the folder 'overlays by category', where the filename is the game category.\n\nYou can find many user-created overlays at https://wwww.reddit.com/r/steamgrid/wiki/overlays .\n\nContinuing without overlays...\n")
	} else {
		fmt.Printf("Loaded %v overlays. \n\nYou can find many user-created overlays at https://wwww.reddit.com/r/steamgrid/wiki/overlays .\n\n", len(overlays))
	}

	fmt.Println("Looking for Steam directory...")
	installationDir, err := GetSteamInstallation()
	if err != nil {
		errorAndExit(err)
	}

	fmt.Println("Loading users...")
	users, err := GetUsers(installationDir)
	if err != nil {
		errorAndExit(err)
	}
	if len(users) == 0 {
		errorAndExit(errors.New("No users found at Steam/userdata. Have you used Steam before in this computer?"))
	}


	nOverlaysApplied := 0
	nDownloaded := 0
	var notFounds []*Game
	var searchedGames []*Game
	var failedGames []*Game
	var errorMessages []string

	for _, user := range users {
		fmt.Println("Loading games for " + user.Name)
		gridDir := filepath.Join(user.Dir, "config", "grid")


		games := GetGames(user)

		fmt.Println("Loading existing images and backups...")
		for _, game := range games {
			overridePath := filepath.Join(filepath.Dir(os.Args[0]), "games")
			LoadExisting(overridePath, gridDir, game)
		}

		// From this point onward we can delete the entire grid/ dir, because all relevant data is loaded in 'games'.
		// This clean unused backups, and game images with different extensions.

		fmt.Println("Creating new grid...")
		newGridDir := gridDir + " new"
		err = os.MkdirAll(filepath.Join(newGridDir, "originals"), 0777)
		if err != nil {
			fmt.Println("Failed to create new empty 'grid':")
			errorAndExit(err)
		}

		i := 0
		for _, game := range games {
			i++

			var name string
			if game.Name != "" {
				name = game.Name
			} else {
				name = "unknown game with id " + game.ID
			}
			fmt.Printf("Processing %v (%v/%v)", name, i, len(games))

			///////////////////////
			// Download if missing.
			///////////////////////
			if game.ImageSource == "" {
				fromSearch, err := DownloadImage(newGridDir, game)
				if err != nil {
					errorAndExit(err)
				}
				if game.ImageSource == "" {
					notFounds = append(notFounds, game)
					fmt.Printf(" not found\n")
					// Game has no image, skip it.
					continue
				} else {
					nDownloaded++
				}

				if fromSearch {
					searchedGames = append(searchedGames, game)
				}
			}
			fmt.Printf(" found from %v\n", game.ImageSource)

			///////////////////////
			// Apply overlay.
			///////////////////////
			err := ApplyOverlay(game, overlays)
			if err != nil {
				print(err.Error(), "\n")
				failedGames = append(failedGames, game)
				errorMessages = append(errorMessages, err.Error())
			}
			if game.OverlayImageBytes != nil {
				nOverlaysApplied++
			} else {
				game.OverlayImageBytes = game.CleanImageBytes
			}

			///////////////////////
			// Save result.
			///////////////////////
			err = BackupGame(newGridDir, game)
			if err != nil {
				errorAndExit(err)
			}
			if game.ImageExt == "" {
				errorAndExit(errors.New("Failed to identify image format."))
			}
			imagePath := filepath.Join(newGridDir, game.ID+game.ImageExt)
			err = ioutil.WriteFile(imagePath, game.OverlayImageBytes, 0666)
			if err != nil {
				fmt.Printf("Failed to write image for %v because: %v\n", game.Name, err.Error())
			}
		}

		fmt.Println("Removing old grid...")
		err = os.RemoveAll(gridDir)
		if err != nil {
			fmt.Println("Failed to remove old directory:")
			errorAndExit(err)
		}

		fmt.Println("Moving new grid to correct location...")
		err = os.Rename(newGridDir, gridDir)
		if err != nil {
			fmt.Println("Failed to move new grid dir to correct location:")
			errorAndExit(err)
		}
	}

	fmt.Printf("\n\n%v images downloaded and %v overlays applied.\n\n", nDownloaded, nOverlaysApplied)
	if len(searchedGames) >= 1 {
		fmt.Printf("%v images were found with a Google search and may not be accurate:\n", len(searchedGames))
		for _, game := range searchedGames {
			fmt.Printf("* %v (steam id %v)\n", game.Name, game.ID)
		}

		fmt.Printf("\n\n")
	}

	if len(notFounds) >= 1 {
		fmt.Printf("%v images could not be found anywhere:\n", len(notFounds))
		for _, game := range notFounds {
			fmt.Printf("- %v (id %v)\n", game.Name, game.ID)
		}

		fmt.Printf("\n\n")
	}

	if len(failedGames) >= 1 {
		fmt.Printf("%v images were found but had errors and could not be overlaid:\n", len(failedGames))
		for i, game := range failedGames {
			fmt.Printf("- %v (id %v) (%v)\n", game.Name, game.ID, errorMessages[i])
		}

		fmt.Printf("\n\n")
	}

	fmt.Println("Open Steam in grid view to see the results!\n\nPress enter to close.")

	bufio.NewReader(os.Stdin).ReadBytes('\n')
}

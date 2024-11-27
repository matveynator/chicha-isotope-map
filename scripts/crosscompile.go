package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {

	// Step 1: Automatically find the main Go file
	goSourceFile, err := findMainGoFile()

	if err != nil {
		log.Fatalf("Error finding main Go file: %v", err)
	}

	// Extract the base name of the source file
	baseName := filepath.Base(goSourceFile)
	executionFile := strings.TrimSuffix(baseName, filepath.Ext(baseName))

	// Get the current Git version
	gitVersion, err := getGitVersion()
	if err != nil {
		log.Fatalf("Error getting Git version: %v", err)
	}
	version := gitVersion
	fmt.Printf("Building version: %s\n", version)

	// Get the root path of the Git repository
	gitRootPath, err := getGitRootPath()
	if err != nil {
		log.Fatalf("Error getting Git root path: %v", err)
	}

	// Set up directories
	binariesPath := filepath.Join(gitRootPath, "binaries", version)
	err = os.MkdirAll(binariesPath, os.ModePerm)
	if err != nil {
		log.Fatalf("Error creating binaries directory: %v", err)
	}

	latestLink := filepath.Join(gitRootPath, "binaries", "latest")
	os.Remove(latestLink)
	err = os.Symlink(version, latestLink)
	if err != nil {
		log.Printf("Warning: Failed to create symlink 'latest': %v", err)
	}

	// Step 4: Build for multiple OS and architectures
	osList := []string{
		"android", "aix", "darwin", "dragonfly", "freebsd",
		"illumos", "ios", "js", "linux", "netbsd",
		"openbsd", "plan9", "solaris", "windows", "wasip1", "zos",
	}

	//osList = []string{ "linux",}

	archList := []string{
		"amd64", "386", "arm", "arm64", "loong64", "mips64",
		"mips64le", "mips", "mipsle", "ppc64",
		"ppc64le", "riscv64", "s390x", "wasm",
	}

	for _, osName := range osList {
		for _, arch := range archList {
			targetOSName := osName
			execFileName := executionFile

			if osName == "windows" {
				execFileName += ".exe"
			} else if osName == "darwin" {
				targetOSName = "mac"
			}

			outputDir := filepath.Join(binariesPath, "no-gui", targetOSName, arch)
			err := os.MkdirAll(outputDir, os.ModePerm)
			if err != nil {
				log.Printf("Error creating output directory %s: %v", outputDir, err)
				continue
			}

			outputPath := filepath.Join(outputDir, execFileName)

			//fmt.Printf("Builgding %s for %s/%s ...\n", goSourceFile, osName, arch)
			ldflags := fmt.Sprintf("-X config.CompileVersion=%s", version)
			buildCmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", outputPath, goSourceFile)
			buildCmd.Env = append(os.Environ(), "GOOS="+osName, "GOARCH="+arch)
			if err := buildCmd.Run(); err != nil {
				// Remove the directory if build fails
				err = os.RemoveAll(outputDir)
				if err != nil {
					log.Printf("Error removing output directory %s: %v", outputDir, err)
				}
				continue
			} else {
				err = os.Chmod(outputPath, 0755)
				if err != nil {
					log.Printf("Error setting permissions on %s: %v", outputPath, err)
				}

				fmt.Printf("Successfully built %s for %s/%s\n", execFileName, osName, arch)
			}
		}
	}

	// Default deployment settings
	deployPath := "/home/files/public_html/" + executionFile + "/"
	remoteHost := "files@files.zabiyaka.net"

	// Step 5: Optional deployment over SSH
	fmt.Print("Do you want to deploy the binaries over SSH? (Y/n): ")
	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	if response == "n" {
		fmt.Println("Deployment skipped.")
	} else {

		var input string

		// Optionally change remoteHost
		fmt.Printf("Default remote host is '%s'. Press Enter to keep it or type a new host: ", remoteHost)
		fmt.Scanln(&input)
		if input != "" {
			remoteHost = input
		}

		// Optionally change deployPath
		fmt.Printf("Default deployment path is '%s'. Press Enter to keep it or type a new path: ", deployPath)
		fmt.Scanln(&input)
		if input != "" {
			deployPath = input
		}

		err = runCommand("rsync", "-avP", "binaries/", fmt.Sprintf("%s:%s", remoteHost, deployPath))
		if err != nil {
			log.Printf("Error deploying binaries: %v", err)
		} else {
			fmt.Println("Deployment completed successfully.")
		}
	}
}

// Helper function to run a command
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Helper function to get the Git root path
func getGitRootPath() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// Helper function to get a cleaner Git version
func getGitVersion() (string, error) {
	// Get a sequential commit count as a "build number"
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	versionNumber := strings.TrimSpace(string(output))

	// Optionally append additional information, such as "-dirty" if there are uncommitted changes
	cmd = exec.Command("git", "status", "--porcelain")
	output, err = cmd.Output()
	if err != nil {
		return "", err
	}
	if len(strings.TrimSpace(string(output))) > 0 {
		versionNumber += "-dirty"
	}

	return versionNumber, nil
}

// Helper function to find the main Go file
func findMainGoFile() (string, error) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		return "", err
	}

	for _, file := range files {
		content, err := ioutil.ReadFile(file)
		if err != nil {
			continue
		}
		if strings.Contains(string(content), "package main") && strings.Contains(string(content), "func main()") {
			return file, nil
		}
	}
	return "", fmt.Errorf("No main Go file found in the current directory")
}


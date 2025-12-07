package main

// crosscompile.go builds binaries across platforms while aligning the
// build number with GitHub Actions run numbers for consistency.
// It fetches the latest run number from GitHub and handles tasks concurrently.
// This approach follows Go proverbs by keeping the code clear and simple.

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// buildJob describes a single compilation task.
// When DuckDB is true we compile with CGO and the duckdb tag.
type buildJob struct {
	osName string
	arch   string
	duckdb bool
}

// buildResult carries the outcome of a buildJob through a channel.
type buildResult struct {
	job buildJob
	err error
}

func main() {

	//download all modules
	goModTidy := exec.Command("go", "mod", "tidy")
	if err := goModTidy.Run(); err != nil {
		fmt.Printf("go mod tidy - failed: %s\n;", err)
	}

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

	//osList = []string{ "darwin",}

	archList := []string{
		"amd64", "386", "arm", "arm64", "loong64", "mips64",
		"mips64le", "mips", "mipsle", "ppc64",
		"ppc64le", "riscv64", "s390x", "wasm",
	}

	for _, osName := range osList {
		for _, arch := range archList {
			// Build the vanilla binary and, when possible, the DuckDB variant in parallel.
			jobs := []buildJob{{osName: osName, arch: arch, duckdb: false}}
			if supportsDuckDB(osName, arch) {
				jobs = append(jobs, buildJob{osName: osName, arch: arch, duckdb: true})
			}

			results := make(chan buildResult, len(jobs))
			for _, job := range jobs {
				go func(j buildJob) {
					err := buildBinary(j, goSourceFile, executionFile, binariesPath, version)
					results <- buildResult{job: j, err: err}
				}(job)
			}

			for completed := 0; completed < len(jobs); {
				select {
				case _ = <-results:
					completed++
					//if res.err != nil {
					//	log.Printf("Error building %s/%s (duckdb=%v): %v", res.job.osName, res.job.arch, res.job.duckdb, res.err)
					//}
				}
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

// buildBinary compiles a single target and writes it into a variant-specific directory.
// When job.duckdb is true, we enable CGO and build with the "duckdb" tag.
func buildBinary(job buildJob, goSourceFile, executionFile, binariesPath, version string) error {
	targetOSName := job.osName
	execFileName := executionFile

	if job.osName == "windows" {
		execFileName += ".exe"
	} else if job.osName == "darwin" {
		targetOSName = "mac"
	}

	variantDir := "no-gui"
	if job.duckdb {
		variantDir = "no-gui-duckdb"
	}

	outputDir := filepath.Join(binariesPath, variantDir, targetOSName, job.arch)
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return fmt.Errorf("creating output directory %s: %w", outputDir, err)
	}

	outputPath := filepath.Join(outputDir, execFileName)
	ldflags := fmt.Sprintf("-X 'main.CompileVersion=%s'", version)

	args := []string{"build", "-ldflags", ldflags}
	if job.duckdb {
		args = append(args, "-tags", "duckdb")
	}
	args = append(args, "-o", outputPath, goSourceFile)
	buildCmd := exec.Command("go", args...)

	env := append(os.Environ(), "GOOS="+job.osName, "GOARCH="+job.arch)
	if job.duckdb {
		env = append(env, "CGO_ENABLED=1")
	} else {
		env = append(env, "CGO_ENABLED=0")
	}
	buildCmd.Env = env

	out, err := buildCmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(outputDir)
		return fmt.Errorf("go build failed: %v: %s", err, strings.TrimSpace(string(out)))
	}

	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("chmod failed on %s: %w", outputPath, err)
	}

	fmt.Printf("Successfully built %s for %s/%s (duckdb=%v)\n", execFileName, job.osName, job.arch, job.duckdb)
	return nil
}

// supportsDuckDB reports whether the given OS and architecture combination can build with DuckDB.
// DuckDB driver is available on Linux/amd64, macOS/amd64, macOS/arm64, and Windows/amd64.
func supportsDuckDB(osName, arch string) bool {
	switch osName {
	case "linux":
		return arch == "amd64"
	case "darwin":
		return arch == "amd64" || arch == "arm64"
	case "windows":
		return arch == "amd64"
	default:
		return false
	}
}

// ----- Git helpers -----
// Helper function to get the Git root path
func getGitRootPath() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// Helper function to get the build number aligned with GitHub Actions
// It tries environment variable, then GitHub API, and falls back to commit count.
// The function uses goroutines and channels to check the run number and dirty state concurrently.
func getGitVersion() (string, error) {
	runChan := make(chan string)
	dirtyChan := make(chan bool)
	errChan := make(chan error, 2)

	go func() {
		if env := os.Getenv("GITHUB_RUN_NUMBER"); env != "" {
			runChan <- env
			return
		}
		n, err := fetchNextRunNumber()
		if err != nil {
			errChan <- err
			return
		}
		runChan <- n
	}()

	go func() {
		cmd := exec.Command("git", "status", "--porcelain")
		output, err := cmd.Output()
		if err != nil {
			errChan <- err
			return
		}
		dirtyChan <- len(strings.TrimSpace(string(output))) > 0
	}()

	var runNumber string
	dirty := false
	for i := 0; i < 2; i++ {
		select {
		case rn := <-runChan:
			runNumber = rn
		case d := <-dirtyChan:
			dirty = d
		case err := <-errChan:
			return "", err
		}
	}

	if runNumber == "" {
		cmd := exec.Command("git", "rev-list", "--count", "HEAD")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		runNumber = strings.TrimSpace(string(output))
	}

	if dirty {
		runNumber += "-dirty"
	}
	return runNumber, nil
}

// ----- File helpers -----
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

// ----- Version helpers -----
// fetchNextRunNumber retrieves the next GitHub Actions run number using the API.
// This ensures local builds share numbering with CI builds.
func fetchNextRunNumber() (string, error) {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	owner, repo, err := parseGitHubRepo(strings.TrimSpace(string(output)))
	if err != nil {
		return "", err
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/release.yml/runs?per_page=1", owner, repo)
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		WorkflowRuns []struct {
			RunNumber int `json:"run_number"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.WorkflowRuns) == 0 {
		return "1", nil
	}
	return strconv.Itoa(result.WorkflowRuns[0].RunNumber + 1), nil
}

// parseGitHubRepo extracts owner and repository from remote URL.
func parseGitHubRepo(remote string) (string, string, error) {
	if strings.HasPrefix(remote, "git@") {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid remote URL")
		}
		remote = parts[1]
	} else if strings.HasPrefix(remote, "https://") || strings.HasPrefix(remote, "http://") {
		u, err := url.Parse(remote)
		if err != nil {
			return "", "", err
		}
		remote = strings.TrimPrefix(u.Path, "/")
	}
	remote = strings.TrimSuffix(remote, ".git")
	parts := strings.Split(remote, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unable to parse owner and repo")
	}
	return parts[0], parts[1], nil
}

package analysis

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GitAnalyzer provides Git repository analysis functionality
type GitAnalyzer struct {
	projectPath string
}

// NewGitAnalyzer creates a new Git analyzer
func NewGitAnalyzer(projectPath string) *GitAnalyzer {
	return &GitAnalyzer{
		projectPath: projectPath,
	}
}

// AnalyzeGitRepository performs comprehensive Git repository analysis
func (ga *GitAnalyzer) AnalyzeGitRepository() (*GitAnalysis, error) {
	// Check if it's a Git repository
	if !ga.isGitRepository() {
		return &GitAnalysis{IsGitRepo: false}, nil
	}

	analysis := &GitAnalysis{
		IsGitRepo: true,
		Metadata:  make(map[string]string),
	}

	// Get basic repository information
	if err := ga.getBasicRepoInfo(analysis); err != nil {
		return analysis, err
	}

	// Get commit activity
	if activity, err := ga.getCommitActivity(); err == nil {
		analysis.CommitActivity = activity
	}

	// Get contributors
	if contributors, err := ga.getContributors(); err == nil {
		analysis.Contributors = contributors
	}

	// Get last commit
	if lastCommit, err := ga.getLastCommit(); err == nil {
		analysis.LastCommit = lastCommit
	}

	// Check for CI/CD
	analysis.HasCI, analysis.CIPlatforms = ga.detectCIPlatforms()

	// Get tags
	if tags, err := ga.getTags(); err == nil {
		analysis.Tags = tags
	}

	// Calculate project age
	if age, err := ga.getProjectAge(); err == nil {
		analysis.ProjectAge = age
	}

	// Get file change statistics
	if fileChanges, err := ga.getFileChangeStats(); err == nil {
		analysis.FileChanges = fileChanges
	}

	return analysis, nil
}

// isGitRepository checks if the project is a Git repository
func (ga *GitAnalyzer) isGitRepository() bool {
	gitDir := filepath.Join(ga.projectPath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}

	// Check if we're in a Git repository (even if .git is in parent)
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = ga.projectPath
	return cmd.Run() == nil
}

// getBasicRepoInfo gets basic repository information
func (ga *GitAnalyzer) getBasicRepoInfo(analysis *GitAnalysis) error {
	// Get remote URL
	if remoteURL, err := ga.runGitCommand("config", "--get", "remote.origin.url"); err == nil {
		analysis.RemoteURL = strings.TrimSpace(remoteURL)
	}

	// Get current branch
	if branch, err := ga.runGitCommand("branch", "--show-current"); err == nil {
		analysis.Branch = strings.TrimSpace(branch)
	}

	return nil
}

// getCommitActivity analyzes commit patterns
func (ga *GitAnalyzer) getCommitActivity() (*CommitActivity, error) {
	activity := &CommitActivity{}

	// Get total commit count
	if totalStr, err := ga.runGitCommand("rev-list", "--count", "HEAD"); err == nil {
		if total, err := strconv.Atoi(strings.TrimSpace(totalStr)); err == nil {
			activity.TotalCommits = total
		}
	}

	// Get recent commits (last 30 days)
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	if recentStr, err := ga.runGitCommand("rev-list", "--count", "--since="+thirtyDaysAgo, "HEAD"); err == nil {
		if recent, err := strconv.Atoi(strings.TrimSpace(recentStr)); err == nil {
			activity.RecentCommits = recent
		}
	}

	// Calculate average commits per week (based on project age)
	if age, err := ga.getProjectAge(); err == nil && age > 0 {
		weeks := float32(age) / 7.0
		if weeks > 0 {
			activity.AveragePerWeek = float32(activity.TotalCommits) / weeks
		}
	}

	// Get most active day of week
	if dayOutput, err := ga.runGitCommand("log", "--format=%ad", "--date=format:%u", "--all"); err == nil {
		activity.MostActiveDay = ga.getMostFrequentDay(dayOutput)
	}

	// Get most active hour
	if hourOutput, err := ga.runGitCommand("log", "--format=%ad", "--date=format:%H", "--all"); err == nil {
		activity.MostActiveHour = ga.getMostFrequentHour(hourOutput)
	}

	return activity, nil
}

// getContributors gets repository contributors
func (ga *GitAnalyzer) getContributors() ([]GitContributor, error) {
	output, err := ga.runGitCommand("shortlog", "-sne", "HEAD")
	if err != nil {
		return nil, err
	}

	var contributors []GitContributor
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse format: "     5	John Doe <john@example.com>"
		re := regexp.MustCompile(`^\s*(\d+)\s+(.+?)\s+<(.+)>$`)
		matches := re.FindStringSubmatch(line)

		if len(matches) == 4 {
			commits, _ := strconv.Atoi(matches[1])
			contributors = append(contributors, GitContributor{
				Name:    matches[2],
				Email:   matches[3],
				Commits: commits,
			})
		}
	}

	// Sort by commit count (descending)
	sort.Slice(contributors, func(i, j int) bool {
		return contributors[i].Commits > contributors[j].Commits
	})

	// Limit to top 10 contributors
	if len(contributors) > 10 {
		contributors = contributors[:10]
	}

	return contributors, nil
}

// getLastCommit gets the last commit information
func (ga *GitAnalyzer) getLastCommit() (*GitCommit, error) {
	// Get commit info in custom format
	format := "--format=%H|%an|%ae|%ad|%s|%n"
	output, err := ga.runGitCommand("log", "-1", format, "--date=iso")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("no commit found")
	}

	parts := strings.Split(lines[0], "|")
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid commit format")
	}

	commit := &GitCommit{
		Hash:    parts[0],
		Author:  parts[1],
		Email:   parts[2],
		Date:    parts[3],
		Message: parts[4],
	}

	// Get files changed in last commit
	if filesStr, err := ga.runGitCommand("diff-tree", "--no-commit-id", "--name-only", "-r", commit.Hash); err == nil {
		commit.FilesChanged = len(strings.Split(strings.TrimSpace(filesStr), "\n"))
	}

	return commit, nil
}

// detectCIPlatforms detects CI/CD platforms
func (ga *GitAnalyzer) detectCIPlatforms() (bool, []string) {
	var platforms []string

	ciFiles := map[string]string{
		".github/workflows":       "GitHub Actions",
		".gitlab-ci.yml":          "GitLab CI",
		".travis.yml":             "Travis CI",
		"circle.yml":              "CircleCI",
		".circleci/config.yml":    "CircleCI",
		"azure-pipelines.yml":     "Azure Pipelines",
		"Jenkinsfile":             "Jenkins",
		".buildkite":              "Buildkite",
		"bitbucket-pipelines.yml": "Bitbucket Pipelines",
		".drone.yml":              "Drone CI",
		"wercker.yml":             "Wercker",
	}

	for file, platform := range ciFiles {
		if _, err := os.Stat(filepath.Join(ga.projectPath, file)); err == nil {
			platforms = append(platforms, platform)
		}
	}

	return len(platforms) > 0, platforms
}

// getTags gets repository tags
func (ga *GitAnalyzer) getTags() ([]string, error) {
	output, err := ga.runGitCommand("tag", "-l", "--sort=-version:refname")
	if err != nil {
		return nil, err
	}

	tags := strings.Split(strings.TrimSpace(output), "\n")
	if len(tags) == 1 && tags[0] == "" {
		return []string{}, nil
	}

	// Limit to latest 10 tags
	if len(tags) > 10 {
		tags = tags[:10]
	}

	return tags, nil
}

// getProjectAge calculates project age in days
func (ga *GitAnalyzer) getProjectAge() (int, error) {
	output, err := ga.runGitCommand("log", "--reverse", "--format=%ad", "--date=iso", "-1")
	if err != nil {
		return 0, err
	}

	firstCommitDate := strings.TrimSpace(output)
	if firstCommitDate == "" {
		return 0, fmt.Errorf("no commits found")
	}

	// Parse ISO date format
	layout := "2006-01-02 15:04:05 -0700"
	firstDate, err := time.Parse(layout, firstCommitDate)
	if err != nil {
		return 0, err
	}

	age := int(time.Since(firstDate).Hours() / 24)
	return age, nil
}

// getFileChangeStats analyzes file change patterns
func (ga *GitAnalyzer) getFileChangeStats() (*FileChangeStats, error) {
	stats := &FileChangeStats{
		LanguageChanges: make(map[string]int),
	}

	// Get most changed files
	output, err := ga.runGitCommand("log", "--name-only", "--pretty=format:", "--all")
	if err != nil {
		return stats, err
	}

	fileChanges := make(map[string]int)
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			fileChanges[line]++

			// Count by language/extension
			ext := strings.ToLower(filepath.Ext(line))
			if ext != "" {
				stats.LanguageChanges[ext]++
			}
		}
	}

	// Convert to sorted slice
	type fileChange struct {
		path    string
		changes int
	}

	var changes []fileChange
	for path, count := range fileChanges {
		changes = append(changes, fileChange{path: path, changes: count})
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].changes > changes[j].changes
	})

	// Get top 10 most changed files
	limit := 10
	if len(changes) < limit {
		limit = len(changes)
	}

	for i := 0; i < limit; i++ {
		stats.MostChangedFiles = append(stats.MostChangedFiles, FileChangeInfo{
			Path:    changes[i].path,
			Changes: changes[i].changes,
			Type:    ga.getFileType(changes[i].path),
		})
	}

	return stats, nil
}

// getMostFrequentDay analyzes day patterns
func (ga *GitAnalyzer) getMostFrequentDay(output string) string {
	dayCount := make(map[string]int)
	dayNames := map[string]string{
		"1": "Monday", "2": "Tuesday", "3": "Wednesday", "4": "Thursday",
		"5": "Friday", "6": "Saturday", "7": "Sunday",
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		day := strings.TrimSpace(scanner.Text())
		if day != "" {
			dayCount[day]++
		}
	}

	maxCount := 0
	mostActiveDay := ""
	for day, count := range dayCount {
		if count > maxCount {
			maxCount = count
			mostActiveDay = day
		}
	}

	if name, exists := dayNames[mostActiveDay]; exists {
		return name
	}
	return ""
}

// getMostFrequentHour analyzes hour patterns
func (ga *GitAnalyzer) getMostFrequentHour(output string) int {
	hourCount := make(map[int]int)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		hourStr := strings.TrimSpace(scanner.Text())
		if hour, err := strconv.Atoi(hourStr); err == nil {
			hourCount[hour]++
		}
	}

	maxCount := 0
	mostActiveHour := 0
	for hour, count := range hourCount {
		if count > maxCount {
			maxCount = count
			mostActiveHour = hour
		}
	}

	return mostActiveHour
}

// getFileType determines file type based on path
func (ga *GitAnalyzer) getFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	filename := strings.ToLower(filepath.Base(path))

	// Config files
	configFiles := map[string]bool{
		"package.json": true, "requirements.txt": true, "go.mod": true,
		"dockerfile": true, "docker-compose.yml": true, "docker-compose.yaml": true,
		"makefile": true, ".gitignore": true, "readme.md": true,
	}

	if configFiles[filename] {
		return "config"
	}

	// By extension
	switch ext {
	case ".js", ".ts", ".py", ".go", ".java", ".rb", ".php", ".rs", ".cpp", ".c", ".cs":
		return "source"
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".env":
		return "config"
	case ".md", ".rst", ".txt":
		return "docs"
	case ".html", ".css", ".scss", ".sass":
		return "web"
	case ".sql":
		return "database"
	case ".sh", ".bash", ".ps1", ".bat":
		return "script"
	default:
		return "other"
	}
}

// runGitCommand executes a git command and returns output
func (ga *GitAnalyzer) runGitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = ga.projectPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

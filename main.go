package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"

	_ "github.com/joho/godotenv/autoload"
)

type (
	programOptions struct {
		GiteaInstance string
		GitHubToken   string
		GiteaToken    string
		GiteaOwner    string
	}

	GitHubRepo struct {
		CloneURL    string `json:"clone_url"`
		Description string `json:"description"`
		Name        string `json:"name"`
		Private     bool   `json:"private"`
	}

	MigrateRepoOptions struct {
		AuthToken   string `json:"auth_token"`
		CloneAddr   string `json:"clone_addr"`
		Description string `json:"description"`
		Mirror      bool   `json:"mirror"`
		Private     bool   `json:"private"`
		RepoName    string `json:"repo_name"`
		RepoOwner   string `json:"repo_owner"`
		Service     string `json:"service"`
		Wiki        bool   `json:"wiki"`
	}

	GiteaRepo struct {
		Id      int64  `json:"id"`
		HtmlUrl string `json:"html_url"`
	}
)

func loadOptions() programOptions {
	return programOptions{
		GiteaInstance: os.Getenv("GITEA_INSTANCE"),
		GitHubToken:   os.Getenv("GITHUB_TOKEN"),
		GiteaToken:    os.Getenv("GITEA_TOKEN"),
		GiteaOwner:    os.Getenv("GITEA_OWNER"),
	}
}

func migrateRepo(options *programOptions) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nEnter the GitHub URL: ")
	url, _ := reader.ReadString('\n')
	url = url[:len(url)-1]

	re := regexp.MustCompile(`github\.com/([^/]+)/([^/]+)`)
	match := re.FindStringSubmatch(url)
	if len(match) != 3 {
		log.Println("Invalid GitHub URL")
		return
	}
	username := match[1]
	repoName := match[2]

	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/%s", username, repoName), nil)
	if err != nil {
		log.Println(err)
		return
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", options.GitHubToken))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	log.Println("Getting GitHub repo info...")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println(err)
			return
		}
	}(resp.Body)

	giteaBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return
	}

	var repo GitHubRepo
	if err := json.Unmarshal(giteaBody, &repo); err != nil {
		log.Println(err)
		return
	}

	log.Printf("Got repo: %s", repo.Name)

	var authToken string
	if repo.Private {
		authToken = options.GitHubToken
	}
	migrateOptions := MigrateRepoOptions{
		AuthToken:   authToken,
		CloneAddr:   repo.CloneURL,
		Description: repo.Description,
		Mirror:      true,
		Private:     repo.Private,
		RepoName:    repo.Name,
		RepoOwner:   options.GiteaOwner,
		Service:     "github",
		Wiki:        true,
	}
	jsonData, err := json.Marshal(migrateOptions)
	if err != nil {
		log.Println(err)
		return
	}

	req, err = http.NewRequest("POST", fmt.Sprintf("https://%s/api/v1/repos/migrate", options.GiteaInstance), bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println(err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", options.GiteaToken))

	log.Println("Creating Gitea repository...")

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println(err)
			return
		}
	}(resp.Body)

	giteaBody, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return
	}

	var giteaRepo GiteaRepo
	if err := json.Unmarshal(giteaBody, &giteaRepo); err != nil {
		log.Println(err)
		return
	}

	if resp.StatusCode == 403 {
		log.Println("Forbidden")
		return
	}

	if resp.StatusCode == 409 {
		log.Println("Repository with this name already exists")
		return
	}

	if resp.StatusCode == 422 {
		log.Println("Wrong input?")
		return
	}

	if giteaRepo.Id == 0 {
		log.Println("Repository creation failed")
		return
	}

	log.Printf("Repository created: %s\n", giteaRepo.HtmlUrl)
}

func main() {
	options := loadOptions()

	fmt.Println("- Welcome to teamigrate -")
	fmt.Printf("GITEA_INSTANCE: %s\n", options.GiteaInstance)
	fmt.Printf("GITEA_OWNER: %s\n", options.GiteaOwner)

	for {
		migrateRepo(&options)
	}

}

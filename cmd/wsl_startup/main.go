package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func main() {
	ipCmd := exec.Command("wsl", "ip", "addr")
	ipOut, err := ipCmd.Output()
	if err != nil {
		panic(err)
	}
	var ip string
	ipRegex := regexp.MustCompile(`inet ((172\.\d+|192\.168)\.\d+\.\d+)\/20`)
	for _, line := range strings.Split(string(ipOut), "\n") {
		match := ipRegex.FindStringSubmatch(line)
		if match != nil {
			ip = match[1]
		}
	}
	if ip == "" {
		panic("Couldn't read ip")
	}
	fmt.Println("IP", ip)

	confPath := "/etc/postgresql/12/main/postgresql.conf"
	sedCommand := fmt.Sprintf(
		`s/listen_addresses = '[0-9]\+\\.[0-9]\+\\.[0-9]\+\\.[0-9]\+'/listen_addresses = '%s'/g`, ip,
	)
	confReplaceCmd := exec.Command("wsl", "sudo", "sed", "-i", sedCommand, confPath)
	err = confReplaceCmd.Run()
	if err != nil {
		panic(err)
	}
	confOutputCmd := exec.Command("wsl", "cat", confPath)
	confOutput, err := confOutputCmd.Output()
	if err != nil {
		panic(err)
	}
	confIPRegex := regexp.MustCompile(`listen_addresses = '([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)'`)
	confIPMatch := confIPRegex.FindStringSubmatch(string(confOutput))
	if confIPMatch == nil {
		panic("Couldn't read conf ip")
	}
	confReplacedIP := confIPMatch[1]
	fmt.Println("Conf IP", confReplacedIP)

	if confReplacedIP == ip {
		startCmd := exec.Command("wsl", "sudo", "service", "postgresql", "start")
		startOut, err := startCmd.Output()
		fmt.Print(string(startOut))
		if err != nil {
			panic(err)
		}
	}

	dbIPPath := "config/wsl_ip.txt"
	err = os.WriteFile(dbIPPath, []byte(ip), 0666)
	if err != nil {
		panic(err)
	}
	dbIP, err := os.ReadFile(dbIPPath)
	if err != nil {
		panic(err)
	}
	fmt.Println("DB IP", string(dbIP))
}

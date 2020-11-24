package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/quipo/statsd"
)

// Config contains the config from the command line parameters
type Config struct {
	APIToken  string
	Format    string
	ServerURL string
}

// Logger for the app
var Logger *log.Logger

func main() {
	setupLogger()

	config, err := readCommandLineArgs()
	if err != nil {
		fmt.Printf("The config was not complete: %v.\nUsage: ./goScrapeAnsibleMetrics --api-token={} --format={} --server-url={}.\n", err)
		os.Exit(0)
	}

	rawMetrics, err := getMetrics(config)
	if err != nil {
		Logger.Println(fmt.Sprintf("There was an error scraping Ansible. Error: %v", err.Error()))
		os.Exit(0)
	}

	Logger.Println(fmt.Sprintf("Received metrics:\n%v", rawMetrics))

	convertMetricsToStatsD(rawMetrics)
}

func readCommandLineArgs() (Config, error) {
	apiToken := flag.String("api-token", "", "API Token for Ansible Tower")
	serverURL := flag.String("server-url", "localhost", "Ansible Tower Server URL")

	flag.Parse()

	if *apiToken == "" {
		Logger.Println("There was no API token provided. An Ansible Tower API key is required")
		return Config{}, fmt.Errorf("There was no API token provided. An Ansible Tower API key is required")
	}

	if *serverURL == "localhost" {
		Logger.Println("There was no Server URL provided. Defaulting to localhost")
	}

	config := Config{
		APIToken:  *apiToken,
		ServerURL: *serverURL,
	}

	return config, nil
}

func setupLogger() {
	// open file for debugging
	logFile, err := os.OpenFile("/tmp/goScrapeAnsibleMetricsStatsD.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		log.Printf("error opening file: %v\n", err)
	}
	defer logFile.Close()
	Logger = log.New(logFile, "", log.LstdFlags)
}

func getMetrics(config Config) (string, error) {
	serverURL := fmt.Sprintf("https://%v/api/v2/metrics/", config.ServerURL)
	// Build the request object
	req, err := http.NewRequest("GET", serverURL, nil)
	if err != nil {
		return "", err
	}

	// Add the API token
	apiTokenField := fmt.Sprintf("Bearer %v", config.APIToken)
	req.Header.Add("Authorization", apiTokenField)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{
		Timeout:   time.Second * 10,
		Transport: tr,
	}

	// Perform the request
	r, err := client.Do(req)
	if err != nil {
		return "", err
	}

	// Check the status code
	if r.StatusCode != 200 {
		return "", fmt.Errorf("Invalid status code from Ansible Tower: %v. ", r.StatusCode)
	}

	// Read in the body
	b, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		return "", fmt.Errorf("Couldn't read the body of the request: %v", err)
	}

	return string(b), nil
}

func convertMetricsToStatsD(rawMetrics string) {
	metrics := strings.Split(rawMetrics, "\n")
	statsClient := makeStatsDClient()

	for _, metric := range metrics {
		if len(metric) > 1 {
			if metric[0] != '#' {
				noQuotes := strings.ReplaceAll(metric, "\"", "")
				cleanMetric := strings.ReplaceAll(noQuotes, "{", ",")
				newMetric := strings.ReplaceAll(cleanMetric, "}", "")

				metricValue := strings.Split(newMetric, " ")
				value, err := strconv.Atoi(metricValue[1])
				if err != nil {
					fmt.Println(fmt.Sprintf("Couldn't convert metric to int: %v\n", metricValue))
					Logger.Printf("Couldn't convert metric to int: %v\n", metricValue)
				}

				fmt.Printf("values are %v, %v\n", metricValue[0], int64(value))
				statsClient.Gauge(metricValue[0], int64(value))

				Logger.Println(fmt.Sprintf("Printed metric: %v", metricValue))
				fmt.Println(fmt.Sprintf("Sent StatsD metric: %v", metricValue))
			}
		}
	}

	// Make sure to close the client and send the messages
	// statsClient.Close()
}

func makeStatsDClient() *statsd.StatsdBuffer {
	prefix := "statsd."
	statsdclient := statsd.NewStatsdClient("localhost:18125", prefix)
	err := statsdclient.CreateSocket()
	if nil != err {
		log.Println(err)
		os.Exit(1)
	}
	interval := time.Second * 2 // aggregate stats and flush every 2 seconds
	stats := statsd.NewStatsdBuffer(interval, statsdclient)
	defer stats.Close()

	return stats
}

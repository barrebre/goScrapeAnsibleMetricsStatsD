package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var conn net.Conn

// Config contains the config from the command line parameters
type Config struct {
	APIToken  string
	Format    string
	ServerURL string
}

func main() {
	config, err := readCommandLineArgs()
	if err != nil {
		fmt.Printf("The config was not complete: %v.\nUsage: ./goScrapeAnsibleMetrics --api-token={} --format={} --server-url={}.\n", err)
		os.Exit(0)
	}

	conn, err = net.Dial("udp", "127.0.0.1:18125")
	if err != nil {
		panic(fmt.Sprintf("Couldn't connect to local statsd listener on port 18125: %v\n", err))
	}

	rawMetrics, err := getMetrics(config)
	if err != nil {
		fmt.Println(fmt.Sprintf("There was an error scraping Ansible. Error: %v", err.Error()))
		os.Exit(0)
	}

	fmt.Printf("Received metrics:\n%v\n", rawMetrics)

	convertMetricsToStatsD(rawMetrics)
}

func readCommandLineArgs() (Config, error) {
	apiToken := flag.String("api-token", "", "API Token for Ansible Tower")
	serverURL := flag.String("server-url", "localhost", "Ansible Tower Server URL")

	flag.Parse()

	if *apiToken == "" {
		fmt.Println("There was no API token provided. An Ansible Tower API key is required")
		return Config{}, fmt.Errorf("There was no API token provided. An Ansible Tower API key is required")
	}

	if *serverURL == "localhost" {
		fmt.Println("There was no Server URL provided. Defaulting to localhost")
	}

	config := Config{
		APIToken:  *apiToken,
		ServerURL: *serverURL,
	}

	return config, nil
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

	for _, metric := range metrics {
		if len(metric) > 1 {
			if metric[0] != '#' {
				// awx_instance_consumed_capacity{hostname="localhost",instance_uuid="98f9ca33-0ec9-4715-bf57-63e91f32a01a"} 0.0
				// awx_instance_consumed_capacity:0|g|#hostname:localhost,...

				metricValueSplit := strings.Split(metric, " ")
				metricString := metricValueSplit[0]

				metricStringParts := strings.Split(metricString, "{")

				metricName := metricStringParts[0]
				metricValue, err := strconv.ParseFloat(metricValueSplit[1], 32)
				if err != nil {
					fmt.Printf("Couldn't convert metric to float: %v\n", metricValue)
				}

				finalMetric := ""
				if len(metricStringParts) > 1 {
					metricDimensions := metricStringParts[1]
					metricDimensionsClean := metricDimensions[0 : len(metricDimensions)-1]
					metricDimensionsParts := strings.Split(metricDimensionsClean, ",")

					dimensionString := "#"
					for _, name := range metricDimensionsParts {
						parts := strings.Split(name, "=")
						dimName := parts[0]
						dimVal := parts[1][1 : len(parts[1])-1]
						dimensionString += fmt.Sprintf("%v:%v,", dimName, dimVal)
					}
					dimString := dimensionString[0 : len(dimensionString)-1]

					finalMetric = fmt.Sprintf("statsd.%v:%v|g|%v", metricName, int(metricValue), dimString)
				} else {
					finalMetric = fmt.Sprintf("statsd.%v:%v|g", metricName, int(metricValue))
				}
				fmt.Println("Final metric is: ", finalMetric)
				fmt.Fprintf(conn, finalMetric)
			}
		}
	}
}

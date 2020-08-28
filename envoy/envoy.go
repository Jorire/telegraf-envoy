// Package envoy is loosely based off of https://github.com/danielnelson/telegraf-plugins
package envoy

import (
	"encoding/json"
	"fmt"
	"math"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
	dac "github.com/xinsnake/go-http-digest-auth-client"
)

const (
	defaultBaseURL                       = "http://envoy"
	defaultResponseTimeout time.Duration = time.Second * 20
	defaultSerialNumber                  = ""
)

//Envoy telegraf plugin declaration
type Envoy struct {
	BaseURL         string        `toml:"base_url"`
	ResponseTimeout time.Duration `toml:"response_timeout"`
	SerialNumber    string        `toml:"serial_number"`
	//local global vars (cf init)
	envoyHTTPclient    *http.Client
	envoyProductionURL *url.URL
	envoyInvertersURL  *url.URL
}

//InvertersData inverters statistics
type InvertersData []struct {
	SerialNumber    string  `json:"serialNumber"`
	LastReportDate  int64   `json:"lastReportDate"`
	DevType         int     `json:"devType"`
	LastReportWatts float64 `json:"lastReportWatts"`
	MaxReportWatts  float64 `json:"maxReportWatts"`
}

//DeviceData data read from Envoy device
type DeviceData struct {
	Production []struct {
		Type             string  `json:"type"`
		ActiveCount      int     `json:"activeCount"`
		ReadingTime      int     `json:"readingTime"`
		WNow             float64 `json:"wNow"`
		WhLifetime       float64 `json:"whLifetime"`
		MeasurementType  string  `json:"measurementType,omitempty"`
		VarhLeadLifetime float64 `json:"varhLeadLifetime,omitempty"`
		VarhLagLifetime  float64 `json:"varhLagLifetime,omitempty"`
		VahLifetime      float64 `json:"vahLifetime,omitempty"`
		RmsCurrent       float64 `json:"rmsCurrent,omitempty"`
		RmsVoltage       float64 `json:"rmsVoltage,omitempty"`
		ReactPwr         float64 `json:"reactPwr,omitempty"`
		ApprntPwr        float64 `json:"apprntPwr,omitempty"`
		PwrFactor        float64 `json:"pwrFactor,omitempty"`
		WhToday          float64 `json:"whToday,omitempty"`
		WhLastSevenDays  float64 `json:"whLastSevenDays,omitempty"`
		VahToday         float64 `json:"vahToday,omitempty"`
		VarhLeadToday    float64 `json:"varhLeadToday,omitempty"`
		VarhLagToday     float64 `json:"varhLagToday,omitempty"`
	} `json:"production"`
	Consumption []struct {
		Type             string  `json:"type"`
		ActiveCount      int     `json:"activeCount"`
		MeasurementType  string  `json:"measurementType"`
		ReadingTime      int     `json:"readingTime"`
		WNow             float64 `json:"wNow"`
		WhLifetime       float64 `json:"whLifetime"`
		VarhLeadLifetime float64 `json:"varhLeadLifetime"`
		VarhLagLifetime  float64 `json:"varhLagLifetime"`
		VahLifetime      float64 `json:"vahLifetime"`
		RmsCurrent       float64 `json:"rmsCurrent"`
		RmsVoltage       float64 `json:"rmsVoltage"`
		ReactPwr         float64 `json:"reactPwr"`
		ApprntPwr        float64 `json:"apprntPwr"`
		PwrFactor        float64 `json:"pwrFactor"`
		WhToday          float64 `json:"whToday"`
		WhLastSevenDays  float64 `json:"whLastSevenDays"`
		VahToday         float64 `json:"vahToday"`
		VarhLeadToday    float64 `json:"varhLeadToday"`
		VarhLagToday     float64 `json:"varhLagToday"`
	} `json:"consumption"`
	Storage []struct {
		Type        string  `json:"type"`
		ActiveCount int     `json:"activeCount"`
		ReadingTime int     `json:"readingTime"`
		WNow        float64 `json:"wNow"`
		WhNow       float64 `json:"whNow"`
		State       string  `json:"state"`
	} `json:"storage"`
}

func init() {
	inputs.Add("envoy", func() telegraf.Input {
		return &Envoy{
			ResponseTimeout: defaultResponseTimeout,
			BaseURL:         defaultBaseURL,
			SerialNumber:    defaultSerialNumber,
		}
	})
}

//createHTTPClient create a reusable HTTP client
func (r *Envoy) createHTTPClient() (*http.Client, error) {
	var envoyHTTPclient *http.Client
	if len(r.SerialNumber) > 6 {
		t := dac.NewTransport("envoy", r.SerialNumber[len(r.SerialNumber)-6:])
		envoyHTTPclient = &http.Client{
			Transport: &t,
			Timeout:   r.ResponseTimeout,
		}
	} else {
		envoyHTTPclient = &http.Client{
			Transport: &http.Transport{},
			Timeout:   r.ResponseTimeout,
		}
	}

	return envoyHTTPclient, nil
}

//Init init method
func (r *Envoy) Init() error {
	var err error

	// Construct & validate Envoy production url
	r.envoyProductionURL, err = url.Parse(r.BaseURL)
	if err != nil {
		return err
	}
	var productionPath *url.URL
	productionPath, err = url.Parse("./production.json")
	if err != nil {
		return err
	}
	r.envoyProductionURL = r.envoyProductionURL.ResolveReference(productionPath)

	// Construct & validate Envoy inverters url
	r.envoyInvertersURL, err = url.Parse(r.BaseURL)
	if err != nil {
		return err
	}
	var invertersPath *url.URL
	invertersPath, err = url.Parse("./api/v1/production/inverters")
	if err != nil {
		return err
	}
	r.envoyInvertersURL = r.envoyProductionURL.ResolveReference(invertersPath)

	// Crea
	// Create an Auhtenticated HTTP client that is re-used for each
	// collection interval
	r.envoyHTTPclient, err = r.createHTTPClient()
	if err != nil {
		return err
	}

	return nil
}

//SampleConfig Sample configuration for the plugin
func (r *Envoy) SampleConfig() string {
	return `
	## Fetch envoy/enphase statistics
	  [inputs.envoy]
	  	## Base Url
		base_url = "http://envoy/"
		## Envoy Serial Number in order to get inverters detailled statistics 
		## (see http://envoy/ )
		serial_number = "xxxxxxxxxxxxx"
  `
}

//Description Plugin description
func (r *Envoy) Description() string {
	return "Read current statistics from envoy/enphase solar panels"
}

//collectGeneralInformations Add global metrics
func (r *Envoy) collectGeneralInformations(acc telegraf.Accumulator, envoyData DeviceData) {
	for _, prod := range envoyData.Production {
		if prod.Type == "inverters" {
			//General informations
			acc.AddFields("inverter",
				map[string]interface{}{
					"count": prod.ActiveCount,
				},
				nil,
				time.Unix(int64(prod.ReadingTime), 0))
		}
	}
}

//collectProduction Add metrics about solar panels production
func (r *Envoy) collectInstantProduction(acc telegraf.Accumulator, envoyData DeviceData) {
	instantProduction := 0.0
	instantConsumption := 0.0
	instantNet := 0.0
	instantImport := 0.0
	instantExport := 0.0

	for _, prod := range envoyData.Production {
		if prod.MeasurementType == "production" {
			if prod.WNow >= 3.0 {
				instantProduction += prod.WNow
			}
		}
	}
	for _, cons := range envoyData.Consumption {
		if cons.MeasurementType == "total-consumption" {
			instantConsumption += cons.WNow
		}
	}
	instantNet = instantProduction - instantConsumption
	if instantNet > 0 {
		instantExport = math.Abs(instantNet)
	} else {
		instantImport = math.Abs(instantNet)
	}

	acc.AddFields("instant-report",
		map[string]interface{}{
			"production":  instantProduction,
			"consumption": instantConsumption,
			"net":         instantNet,
			"import":      instantImport,
			"export":      instantExport,
		},
		map[string]string{
			"envoy": r.SerialNumber,
			"type":  "instant",
		})
}

//collectProduction Add metrics about solar panels production
func (r *Envoy) collectTodayProduction(acc telegraf.Accumulator, envoyData DeviceData) {
	todayProduction := 0.0
	todayConsumption := 0.0
	todayNet := 0.0
	todayImport := 0.0
	todayExport := 0.0

	for _, prod := range envoyData.Production {
		if prod.MeasurementType == "production" {
			todayProduction += prod.WhToday
		}
	}
	for _, cons := range envoyData.Consumption {
		if cons.MeasurementType == "total-consumption" {
			todayConsumption += cons.WhToday
		}
	}
	todayNet = todayProduction - todayConsumption
	if todayNet > 0 {
		todayExport = math.Abs(todayNet)
	} else {
		todayImport = math.Abs(todayNet)
	}

	acc.AddFields("today-report",
		map[string]interface{}{
			"production":  todayProduction,
			"consumption": todayConsumption,
			"net":         todayNet,
			"import":      todayImport,
			"export":      todayExport,
		},
		map[string]string{
			"envoy": r.SerialNumber,
			"type":  "today",
		})
}

//collectNetConsumption Add metrics about solar panels production
func (r *Envoy) collectNetConsumption(acc telegraf.Accumulator, envoyData DeviceData) {
	todayNet := 0.0
	instantNet := 0.0
	instantImport := 0.0
	instantExport := 0.0
	todayImport := 0.0
	todayExport := 0.0
	for _, prod := range envoyData.Consumption {
		if prod.MeasurementType == "net-consumption" {
			instantNet += prod.WNow
			todayNet += prod.WhToday
		}
	}
	if instantNet > 0 {
		instantExport = math.Abs(instantNet)
	} else {
		instantImport = math.Abs(instantNet)
	}
	if todayNet > 0 {
		todayExport = math.Abs(todayNet)
	} else {
		todayImport = math.Abs(todayNet)
	}
	acc.AddFields("net-report",
		map[string]interface{}{
			"net":    instantNet,
			"import": instantImport,
			"export": instantExport,
		},
		map[string]string{
			"envoy": r.SerialNumber,
			"type":  "instant",
		})
	acc.AddFields("net-report",
		map[string]interface{}{
			"net":    instantNet,
			"import": todayImport,
			"export": todayExport,
		},
		map[string]string{
			"envoy": r.SerialNumber,
			"type":  "today",
		})
}

//collectNetConsumption Add metrics about solar panels production
func (r *Envoy) collectInvertersData(acc telegraf.Accumulator, invertersData InvertersData) {
	before30 := time.Now().Add(-30 * time.Minute)
	for _, inverterData := range invertersData {
		var status int
		reportDate := time.Unix(inverterData.LastReportDate, 0)
		if reportDate.After(before30) {
			status = 1
		} else {
			status = 0
		}
		instant := 0.0
		if inverterData.LastReportWatts >= 3.0 {
			instant = inverterData.LastReportWatts
		}

		acc.AddFields("inverter",
			map[string]interface{}{
				"instant": instant,
				"today":   inverterData.MaxReportWatts,
				"status":  status,
			},
			map[string]string{
				"serialNumber": inverterData.SerialNumber,
				"envoy":        r.SerialNumber,
			})
	}
}

//Gather fetch Envoy data (poll_interval parameter to control frequency)
func (r *Envoy) Gather(acc telegraf.Accumulator) error {
	envoyData, err := r.GatherProductionData()

	if err != nil {
		return err
	}

	if envoyData != nil {
		r.collectGeneralInformations(acc, *envoyData)
		r.collectInstantProduction(acc, *envoyData)
		r.collectTodayProduction(acc, *envoyData)
		r.collectNetConsumption(acc, *envoyData)
	} else {
		return fmt.Errorf("No data gathered")
	}

	invertersData, err := r.GatherInvertersData()
	if err != nil {
		return err
	}

	if invertersData != nil {
		r.collectInvertersData(acc, *invertersData)
	}

	return nil
}

//GatherProductionData get Envoy Data using http request to
func (r *Envoy) GatherProductionData() (*DeviceData, error) {
	url := r.envoyProductionURL.String()
	resp, err := r.envoyHTTPclient.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP status %s", url, resp.Status)
	}

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	if mediaType != "application/json" {
		return nil, fmt.Errorf("%s returned unexpected content type %s", url, mediaType)
	}

	dec := json.NewDecoder(resp.Body)
	deviceData := &DeviceData{}
	if err := dec.Decode(deviceData); err != nil {
		return nil, fmt.Errorf("error while decoding JSON response: %s", err)
	}
	return deviceData, nil
}

//GatherInvertersData get Inverters Data using http request to
func (r *Envoy) GatherInvertersData() (*InvertersData, error) {
	if len(r.SerialNumber) < 6 {
		return nil, nil
	}

	url := r.envoyInvertersURL.String()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.envoyHTTPclient.Transport.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("%s returned HTTP status %s", url, resp.Status)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP status %s", url, resp.Status)
	}

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	if mediaType != "application/json" {
		return nil, fmt.Errorf("%s returned unexpected content type %s", url, mediaType)
	}

	dec := json.NewDecoder(resp.Body)
	invertersData := &InvertersData{}
	if err := dec.Decode(invertersData); err != nil {
		return nil, fmt.Errorf("error while decoding JSON response: %s", err)
	}
	return invertersData, nil
}

package tibber

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const subscriptionEndpoint = "v1-beta/gql/subscriptions"
const tibberHost = "websocket-api.tibber.com"

const (
	StreamStateConnected    = "CONNECTED"
	StreamStateDisconnected = "DISCONNECTED"
)

// MsgChan for reciving messages
type MsgChan chan *StreamMsg

// StreamMsg for streams
type StreamMsg struct {
	Type    string  `json:"type"`
	ID      string  `json:"id"`
	Payload Payload `json:"payload"`
}

type StreamState struct {
	State string
	Err   error
}

// Payload in StreamMsg
type Payload struct {
	Data Data `json:"data"`
}

// Data in Payload
type Data struct {
	LiveMeasurement LiveMeasurement `json:"liveMeasurement"`
}

// LiveMeasurement in data payload
type LiveMeasurement struct {
	Timestamp              time.Time `json:"timestamp"`
	Power                  float64   `json:"power"`
	LastMeterConsumption   float64   `json:"lastMeterConsumption"`
	LastMeterProduction    float64   `json:"lastMeterProduction"`
	AccumulatedConsumption float64   `json:"accumulatedConsumption"`
	AccumulatedCost        float64   `json:"accumulatedCost"`
	AccumulatedProduction  float64   `json:"accumulatedProduction"`
	AccumulatedReward      float64   `json:"accumulatedReward"`
	MinPower               float64   `json:"minPower"`
	AveragePower           float64   `json:"averagePower"`
	MaxPower               float64   `json:"maxPower"`
	PowerProduction        float64   `json:"powerProduction"`
	MinPowerProduction     float64   `json:"minPowerProduction"`
	MaxPowerProduction     float64   `json:"maxPowerProduction"`
	VoltagePhase1          float64   `json:"voltagePhase1"`
	VoltagePhase2          float64   `json:"voltagePhase2"`
	VoltagePhase3          float64   `json:"voltagePhase3"`
	CurrentPhase1          float64   `json:"currentPhase1"`
	CurrentPhase2          float64   `json:"currentPhase2"`
	CurrentPhase3          float64   `json:"currentPhase3"`
}

// IsExtended returns whether the report is normal or extended.
// In an extended report we would have at least one phase information
func (m *LiveMeasurement) IsExtended() bool {
	return m.CurrentPhase1 > 0 || m.CurrentPhase2 > 0 || m.CurrentPhase3 > 0
}

// HasPower returns true if the report contains power measurement
func (m *LiveMeasurement) HasPower() bool {
	return m.Power > 0
}

// HasProductionOrConsumptionPower return true if measurement contains values
func (m *LiveMeasurement) HasProductionOrConsumptionPower() bool {
	return m.Power > 0 || m.PowerProduction > 0
}

// AsFloatMap returns the LiveMeasurement struct as a float map
func (m *LiveMeasurement) AsFloatMap() map[string]float64 {
	return map[string]float64{
		"p_import":      m.Power,
		"e_import":      m.LastMeterConsumption,
		"e_export":      m.LastMeterProduction,
		"last_e_import": m.AccumulatedConsumption,
		"last_e_export": m.AccumulatedProduction,
		"p_import_min":  m.MinPower,
		"p_import_avg":  m.AveragePower,
		"p_import_max":  m.MaxPower,
		"p_export":      m.PowerProduction,
		"p_export_min":  m.MinPowerProduction,
		"p_export_max":  m.MaxPowerProduction,
		"u1":            m.VoltagePhase1,
		"u2":            m.VoltagePhase2,
		"u3":            m.VoltagePhase3,
		"i1":            m.CurrentPhase1,
		"i2":            m.CurrentPhase2,
		"i3":            m.CurrentPhase3,
	}
}

// Stream for subscribing to Tibber pulse
type Stream struct {
	Token           string
	ID              string
	isRunning       bool
	initialized     bool
	client          *websocket.Conn
	stateReportChan chan StreamState
}

func (ts *Stream) StateReportChan() chan StreamState {
	return ts.stateReportChan
}

// NewStream with id and token
func NewStream(id, token string) *Stream {
	ts := Stream{
		ID:              id,
		Token:           token,
		isRunning:       true,
		initialized:     false,
		stateReportChan: make(chan StreamState),
	}
	return &ts
}

// Listen starts the stream and listens for messages, will reconnect until isRunning is set to false.
func (ts *Stream) Listen(outputChan MsgChan) {
	for ts.isRunning {
		err := ts.connect()
		if err != nil {
			log.WithError(err).Error("<TibberStream> Could not connect to websocket")
			time.Sleep(time.Second * 7) // trying to repair the connection
		} else {
			log.Info("<TibberStream> Connected")
			break // connection was made
		}
	}

	for ts.isRunning {
		ts.msgLoop(outputChan)
		log.Error("<TibberStream> Restarting msg router")
	}
}

// StartSubscription init connection and subscribes to home id
//
// Deprecated: Uses go rutin to spawn of unmanaged processes
func (ts *Stream) StartSubscription(outputChan MsgChan) error {
	go ts.Listen(outputChan)
	return nil
}

func (ts *Stream) reportState(state string, err error) {
	st := StreamState{
		State: state,
		Err:   err,
	}
	select {
	case ts.stateReportChan <- st:
	default:
		log.Debug("<TibberStream> No error liste")
	}

}

func (ts *Stream) msgLoop(outputChan MsgChan) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("<TibberStream> Process CRASHED with error: ", r)
			time.Sleep(1 * time.Minute)
		}
		if ts.client != nil {
			ts.client.Close()
		}
	}()
	var unknownErrorCounter int
	for ts.isRunning {
		if !ts.initialized {
			ts.sendInitMsg()
		}
		tm := StreamMsg{}
		err := ts.client.ReadJSON(&tm)
		if err != nil {
			if isWsCloseError(err) {
				log.WithError(err).Error("<TibberStream> CloseError, Reconnecting after 10 seconds")
				ts.reportState(StreamStateDisconnected, err)
				time.Sleep(time.Second * 10) // trying to repair the connection
				err = ts.connect()
				if err != nil {
					log.WithError(err).Error("<TibberStream> Could not connect to websocket")
					time.Sleep(time.Second * 30)
				}
				continue
			} else {
				unknownErrorCounter++
				log.WithError(err).Error("<TibberStream> Unknown error while reading data from WS")
				ts.reportState(StreamStateDisconnected, err)
				time.Sleep(time.Second * 20)
				if unknownErrorCounter > 10 {
					ts.client.Close()
					err = ts.connect()
					if err != nil {
						log.WithError(err).Error("<TibberStream> Could not connect to websocket")
						time.Sleep(time.Second * 60)
					}
				}
				continue
			}
		} else {

			unknownErrorCounter = 0
			switch tm.Type {
			case "connection_ack":
				log.Info("<TibberStream> Init success")
				ts.initialized = true
				ts.sendSubMsg()

			case "subscription_success":
				log.Info("<TibberStream> Subscription success")

			case "next":
				outputChan <- &tm

			case "subscription_fail":
				err := fmt.Errorf("subscription failed")
				log.WithError(err).Error("<TibberStream>")
				ts.reportState(StreamStateDisconnected, err)

			default:
				log.Info("<TibberStream> Unexpected message type :", tm.Type)
			}
		}
	}
	log.Debug("<TibberStream> Stopping")
}
func isWsCloseError(err error) bool {
	return websocket.IsCloseError(err,
		websocket.CloseGoingAway,
		websocket.CloseAbnormalClosure,
		websocket.CloseNormalClosure,
		websocket.CloseProtocolError,
		websocket.CloseUnsupportedData,
		websocket.CloseNoStatusReceived,
		websocket.CloseInvalidFramePayloadData,
		websocket.ClosePolicyViolation,
		websocket.CloseMessageTooBig,
		websocket.CloseMandatoryExtension,
		websocket.CloseInternalServerErr,
		websocket.CloseServiceRestart,
		websocket.CloseTryAgainLater,
		websocket.CloseTLSHandshake)
}

func (ts *Stream) connect() error {
	ts.initialized = false
	defer func() {
		if r := recover(); r != nil {
			log.Error("<TibberStream> ID: ", ts.ID, " - Process CRASHED with error : ", r)
			time.Sleep(time.Minute * 1)
		}
	}()
	u := url.URL{Scheme: "wss", Host: tibberHost, Path: subscriptionEndpoint}
	log.Infof("<TibberStream> Connecting to %s", u.String())
	var err error
	for {
		reqHeader := make(http.Header)
		reqHeader.Add("Sec-WebSocket-Protocol", "graphql-transport-ws")
		reqHeader.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")

		ts.client, _, err = websocket.DefaultDialer.Dial(u.String(), reqHeader)
		if err != nil {
			log.Error("<TibberStream> Dial error", err)
			time.Sleep(time.Second * 2)
		} else {
			log.Info("<TibberStream> WS Client is connected - ID: ", ts.ID, " error: ", err)
			ts.isRunning = true
			ts.reportState(StreamStateConnected, err)
			return nil
		}
	}
}

// Stop stops stream
func (ts *Stream) Stop() {
	log.Debug("<TibberWsClient> setting isRunning to false")
	ts.isRunning = false
}

func (ts *Stream) sendInitMsg() {
	init := `{"type":"connection_init","payload":{"token":"` + ts.Token + `"}}`
	ts.client.WriteMessage(websocket.TextMessage, []byte(init))
}

func jsonEscape(i string) string {
	b, err := json.Marshal(i)
	if err != nil {
		panic(err)
	}
	// Trim the beginning and trailing " character
	return string(b[1 : len(b)-1])
}

func (ts *Stream) sendSubMsg() {
	homeID := ts.ID
	var subscriptionQuery = fmt.Sprintf(`
	subscription {
		liveMeasurement(homeId:"%s") {
			timestamp
			power
			lastMeterConsumption
			lastMeterProduction
			accumulatedConsumption
			accumulatedCost
			accumulatedProduction
			accumulatedReward
			minPower
			averagePower
			maxPower
			powerProduction
			minPowerProduction
			maxPowerProduction
			voltagePhase1
			voltagePhase2
			voltagePhase3
			currentPhase1
			currentPhase2
			currentPhase3
		}
	}`,
		homeID)

	sub := fmt.Sprintf(`
	{
		"payload": {
			"query": "%s",
			"variables":null,
			"extensions":null
		},
		"type":"subscribe",
		"id":"0"
	}`, jsonEscape(subscriptionQuery))

	log.Debug("Subscribe with query", sub)
	err := ts.client.WriteMessage(websocket.TextMessage, []byte(sub))
	if err != nil {
		log.WithError(err).Error("<TibberStream> Error sending subscription message")
	}
}

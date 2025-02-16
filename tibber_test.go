package tibber

import (
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type TestConfig struct {
	Endpoint string `yaml:"endpoint"`
	Token    string `yaml:"token"`
	HomeID   string `yaml:"homeId"`
}

// load TestConfig from yaml file
func loadTestConfig() TestConfig {
	var tc TestConfig
	bytes, err := os.ReadFile("test-config.yaml")
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(bytes, &tc)
	if err != nil {
		panic(err)
	}
	return tc
}

func TestGetHomes(t *testing.T) {
	conf := loadTestConfig()
	tc := NewClient(conf.Token)
	homes, err := tc.GetHomes()
	if err == nil {
		t.Logf("GetHomes: %v", homes)
	} else {
		t.Fatalf("GetHomes: %v, %v", err, homes)
	}
}

func TestSubscriptionFetch(t *testing.T) {
	conf := loadTestConfig()
	tc := NewClient(conf.Token)
	subscription, _ := tc.GetSubscriptionURL()
	t.Logf("SubscriptionUrl: %s", subscription)
}

func TestGetHomeById(t *testing.T) {
	conf := loadTestConfig()
	tc := NewClient(conf.Token)
	t.Logf("HomeID: %s", conf.HomeID)
	home, _ := tc.GetHomeById(conf.HomeID)
	if home.ID == "" {
		t.Fatalf("GetHomeById: %s %v", conf.HomeID, home)
	}
}

// func TestPush(t *testing.T) {
// 	token := string(helperLoadBytes(t, "token.txt"))
// 	tc := NewClient(token)
// 	_, err := tc.SendPushNotification("Golang Test", "Running golang test")
// 	if err != nil {
// 		t.Fatalf("Push: %v", err)
// 	}
// }

func TestStreams(t *testing.T) {
	var msgCh MsgChan
	testConfig := loadTestConfig()
	token := testConfig.Token
	homeID := testConfig.HomeID
	t.Logf("homeID: %s", homeID)
	stream := NewStream(homeID, token)
	err := stream.StartSubscription(msgCh)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	select {
	case msg := <-msgCh:
		t.Log(msg)
	case <-time.After(time.Second * 7):
		break
	}
	stream.Stop()
}

func TestGetCurrentPrice(t *testing.T) {
	conf := loadTestConfig()
	tc := NewClient(conf.Token)
	priceInfo, _ := tc.GetCurrentPrice(conf.HomeID)
	if priceInfo.Level == "" {
		t.Fatalf("GetCurrentPrice: %v", priceInfo)
	} else {
		t.Logf("GetCurrentPrice: %v", priceInfo)
	}
}

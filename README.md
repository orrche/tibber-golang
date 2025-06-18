# tibber-golang  
Limited implementation of the Tibber API in golang  
[developer.tibber.com](https://developer.tibber.com)  

**Possibilities:**
* Get a list of homes with id, name, meterID and features
* Send push notification from the Tibber App
* Subscribe to data from Tibber Pulse


Basic test:
```bash
go test ./...
```

With push notifications:
```bash
go test -tags=tibberpush ./...
```


## Usage

```go
package main

import (
	"fmt"

	tibber "github.com/tskaard/tibber-golang"
)
const token = "<Tibber token>"

func main() {
	msgChannal := make(tibber.MsgChan)
	client := tibber.NewClient(token)
	homes, err := client.GetHomes()
	if err != nil {
		panic("Can not get homes from Tibber")
	}

	for _, home := range homes {
		fmt.Println(home.ID)
		if home.Features.RealTimeConsumptionEnabled {
			stream := tibber.NewStream(home.ID, token)
			go stream.Listen(msgChannal)
		}
	}

	_, err = h.tibber.SendPushNotification("Tibber-Golang", "Message from GO")
	if err != nil {
		panic("Push failed")
	}
	for msg := range msgChannal {
		fmt.Printf("%s - Current consumption: %f KW\n", msg.Payload.Data.LiveMeasurement.Timestamp.String(), msg.Payload.Data.LiveMeasurement.Power/1000)
	}
}
```

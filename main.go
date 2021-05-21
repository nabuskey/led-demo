package main

import (
	"bytes"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var nl Nanoleaf
var db dynamoDB

//go:embed frontend/index.html
var indexHtml []byte

func main() {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	nl.New()
	db.New()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html")
		w.Write(indexHtml)
	})
	//mux.Handle("/static/", http.StripPrefix("/frontend/", fs))
	mux.HandleFunc("/toggle", toggle)
	mux.HandleFunc("/color", setColor)
	mux.HandleFunc("/brightness", setBrightness)
	fmt.Println("started server on 4000")
	err := http.ListenAndServe(":4000", mux)
	if err != nil {
		log.Fatal(err)
	}
}

func toggle(w http.ResponseWriter, r *http.Request) {
	nl.OnOff()
}

func setColor(w http.ResponseWriter, r *http.Request) {
	//if r.Method != http.MethodPut {
	//	w.WriteHeader(http.StatusBadRequest)
	//	w.Write([]byte("bad request"))
	//}
	var rb StaticColorRequest
	err := json.NewDecoder(r.Body).Decode(&rb)
	if err != nil {
		fmt.Printf("error while decoding request body: %v \n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if rb.Color == "" {
		fmt.Println("color not set")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = nl.setStaticColors(rb.Color)
	if err != nil {
		fmt.Printf("error while processing request: %v \n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func setBrightness(w http.ResponseWriter, r *http.Request) {
	var b BrightnessHttpRequest
	err := json.NewDecoder(r.Body).Decode(&b)
	if err != nil {
		fmt.Printf("error while decoding request body: %v \n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	bi, err := strconv.Atoi(b.Value)
	if err != nil {
		fmt.Printf("invalid int specified: %v \n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if bi < 0 || bi > 100 {
		fmt.Printf("invalid int range specified: %v \n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	nl.brightness(bi)
}

type Nanoleaf struct {
	Address   string
	Port      string
	AuthToken string
	Url       string
}

func (n *Nanoleaf) New() {
	n.Address = os.Getenv("NL_ADDRESS")
	n.Port = os.Getenv("NL_PORT")
	n.AuthToken = os.Getenv("NL_TOKEN")
	n.Url = fmt.Sprintf("https://%v:%v/api/v1/%v", n.Address, n.Port, n.AuthToken)
}

func (n *Nanoleaf) setStaticColors(name string) error {
	resp, err := http.Get(fmt.Sprintf("%v/panelLayout/layout", n.Url))
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("error")
	}
	var panels NanoleafPanels
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&panels)
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("error2")
	}
	var intp int
	panelIds := make([]string, panels.NumPanels)
	for i := range panels.PositionData {
		id := panels.PositionData[i].PanelID
		if id != 0 {
			panelIds[intp] = strconv.Itoa(id)
			intp++
		}
	}
	colorValue, err := db.GetColorValue(strings.ToLower(name))
	if err != nil {
		return err
	}
	var sbuilder strings.Builder
	sbuilder.WriteString(fmt.Sprintf("%v ", strconv.Itoa(len(panelIds))))
	for i := range panelIds {
		if panelIds[i] != "" {
			sbuilder.WriteString(fmt.Sprintf("%v 1 %v 0 20 ", panelIds[i], colorValue))
		}
	}
	reqData := NanoleafStaticColorRequest{}
	reqData.Write.Command = "display"
	reqData.Write.AnimType = "static"
	reqData.Write.AnimData = sbuilder.String()
	reqData.Write.Loop = false
	reqData.Write.Palette = []string{}
	fmt.Println(reqData.Write.AnimData)

	req, err := json.Marshal(reqData)
	if err != nil {
		fmt.Printf("unmarshal error: %v", err)
		return err
	}
	err = n.putRequest("effects", req)
	if err != nil {
		fmt.Printf("error when doing PUT: %v", err)
	}
	return nil
}

func (n *Nanoleaf) OnOff() {
	resp, err := http.Get(fmt.Sprintf("%v/state/on", n.Url))
	if err != nil {
		fmt.Println(err)
		return
	}
	var state NanoleafState
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&state)
	if err != nil {
		fmt.Println(err)
		return
	}

	request := NanoleafStateRequest{}
	if state.Value {
		req, err := json.Marshal(request)
		if err != nil {
			fmt.Println("error while converting to []byte")
			fmt.Println(err)
			return
		}
		err = n.putRequest("state", req)
		if err != nil {
			fmt.Println(err)
			return
		}
		return
	}
	request.On.Value = true
	req, err := json.Marshal(request)
	if err != nil {
		fmt.Println("error while converting to []byte")
		fmt.Println(err)
		return
	}
	err = n.putRequest("state", req)
	if err != nil {
		fmt.Println(err)
		return
	}
	return
}

func (n *Nanoleaf) brightness(value int) {
	brightness := BrightnessRequest{
		struct {
			Value    int `json:"value"`
			Duration int `json:"duration"`
		}{Value: value, Duration: 1},
	}
	req, err := json.Marshal(brightness)
	if err != nil {
		fmt.Println("error while converting to []byte")
		fmt.Println(err)
		return
	}
	err = n.putRequest("state", req)
	if err != nil {
		fmt.Println(err)
	}
}

func (n *Nanoleaf) putRequest(path string, req []byte) error {
	client := &http.Client{}
	httpReq, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%v/%v", n.Url, path), bytes.NewBuffer(req))
	if err != nil {
		fmt.Println("error while creating client")
		fmt.Println(err)
		return err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Println("error while making request")
		fmt.Println(err)
		return err
	}
	if resp.StatusCode != 204 {
		fmt.Println("received an invalid response")
		fmt.Println(resp.Status)
	}
	return nil
}

type NanoleafStateRequest struct {
	On struct {
		Value bool `json:"value"`
	} `json:"on"`
}

type NanoleafState struct {
	Value bool `json:"value"`
}

type NanoleafPanels struct {
	NumPanels    int `json:"numPanels"`
	SideLength   int `json:"sideLength"`
	PositionData []struct {
		PanelID   int `json:"panelId"`
		X         int `json:"x"`
		Y         int `json:"y"`
		O         int `json:"o"`
		ShapeType int `json:"shapeType"`
	} `json:"positionData"`
}

type NanoleafStaticColorRequest struct {
	Write struct {
		Command  string   `json:"command"`
		AnimType string   `json:"animType"`
		AnimData string   `json:"animData"`
		Loop     bool     `json:"loop"`
		Palette  []string `json:"palette"`
	} `json:"write"`
}

type StaticColorRequest struct {
	Color string `json:"color"`
}

type BrightnessRequest struct {
	Brightness struct {
		Value    int `json:"value"`
		Duration int `json:"duration"`
	} `json:"brightness"`
}
type BrightnessHttpRequest struct {
	Value string `json:"value"`
}

// Copyright 2023 Tamás Gulácsi.
// Copyright 2020 Manuel. https://manuel-io.github.io/blog/2020/03/29/query-minidlna-to-list-media-files/
//

package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func main() {
	if err := Main(); err != nil {
		log.Fatal("ERROR:", err)
	}
}

func Main() error {
	flagMiniDLNA := flag.String("minidlna", "http://127.0.0.1:8200", "MiniDLNA server address")
	flag.Parse()

	log.Println("Listening on", flag.Arg(0), "...")
	return http.ListenAndServe(flag.Arg(0), handler{baseURL: *flagMiniDLNA})
}

type handler struct{ baseURL string }

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	root, err := getRootDesc(h.baseURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	cpath := root.ContentPath()
	dl, err := root.post(cpath, getObjectID("0"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "max-age=300")
	io.WriteString(w, "<!doctype html>")
	for _, container := range dl.Containers {
		if err := ctx.Err(); err != nil {
			log.Println(err)
			return
		}
		fl, err := root.post(cpath, getObjectID(container.ID))
		if err != nil {
			printErr(err).Render(ctx, w)
			continue
		}
		for _, folder := range fl.Containers {
			ff, err := root.post(cpath, getObjectID(folder.ID))
			if err != nil {
				printErr(err).Render(ctx, w)
				continue
			}
			if len(ff.Items) == 0 {
				continue
			}
			printItems(folder, ff.Items).Render(ctx, w)
		}
	}
	io.WriteString(w, "</html")
}

const (
	contentType = "text/xml; charset=utf-8"
	soapAction  = "urn:schemas-upnp-org:service:ContentDirectory:1#Browse"
)

func getObjectID(index string) string {
	var buf strings.Builder
	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?>
        <s:Envelope xmlns:ns0="urn:schemas-upnp-org:service:ContentDirectory:1" xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
          <s:Body>
            <ns0:Browse>
              <ObjectID>`)
	xml.EscapeText(&buf, []byte(index))
	buf.WriteString(`</ObjectID>
              <BrowseFlag>BrowseDirectChildren</BrowseFlag>
              <Filter>*</Filter>
            </ns0:Browse>
          </s:Body>
        </s:Envelope>
      </xml>`)
	return buf.String()
}

// Root was generated 2023-05-13 18:18:47 by https://xml-to-go.github.io/ in Ukraine.
type Root struct {
	XMLName     xml.Name `xml:"root" json:"root,omitempty"`
	baseURL     string   `xml:"-"`
	Text        string   `xml:",chardata" json:"text,omitempty"`
	Xmlns       string   `xml:"xmlns,attr" json:"xmlns,omitempty"`
	SpecVersion struct {
		Text  string `xml:",chardata" json:"text,omitempty"`
		Major string `xml:"major"`
		Minor string `xml:"minor"`
	} `xml:"specVersion" json:"specversion,omitempty"`
	Device struct {
		Text             string `xml:",chardata" json:"text,omitempty"`
		DeviceType       string `xml:"deviceType"`
		FriendlyName     string `xml:"friendlyName"`
		Manufacturer     string `xml:"manufacturer"`
		ManufacturerURL  string `xml:"manufacturerURL"`
		ModelDescription string `xml:"modelDescription"`
		ModelName        string `xml:"modelName"`
		ModelNumber      string `xml:"modelNumber"`
		ModelURL         string `xml:"modelURL"`
		SerialNumber     string `xml:"serialNumber"`
		UDN              string `xml:"UDN"`
		XDLNADOC         struct {
			Text string `xml:",chardata" json:"text,omitempty"`
			Dlna string `xml:"dlna,attr" json:"dlna,omitempty"`
		} `xml:"X_DLNADOC" json:"x_dlnadoc,omitempty"`
		PresentationURL string `xml:"presentationURL"`
		IconList        struct {
			Text string `xml:",chardata" json:"text,omitempty"`
			Icon []struct {
				Text     string `xml:",chardata" json:"text,omitempty"`
				Mimetype string `xml:"mimetype"`
				Width    string `xml:"width"`
				Height   string `xml:"height"`
				Depth    string `xml:"depth"`
				URL      string `xml:"url"`
			} `xml:"icon" json:"icon,omitempty"`
		} `xml:"iconList" json:"iconlist,omitempty"`
		ServiceList struct {
			Text    string `xml:",chardata" json:"text,omitempty"`
			Service []struct {
				Text        string `xml:",chardata" json:"text,omitempty"`
				ServiceType string `xml:"serviceType"`
				ServiceId   string `xml:"serviceId"`
				ControlURL  string `xml:"controlURL"`
				EventSubURL string `xml:"eventSubURL"`
				SCPDURL     string `xml:"SCPDURL"`
			} `xml:"service" json:"service,omitempty"`
		} `xml:"serviceList" json:"servicelist,omitempty"`
	} `xml:"device" json:"device,omitempty"`
}

func getRootDesc(baseURL string) (Root, error) {
	resp, err := http.Get(baseURL + "/rootDesc.xml")
	if err != nil {
		return Root{}, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return Root{}, err
	}
	var root Root
	if err = xml.Unmarshal(b, &root); err != nil {
		return Root{}, fmt.Errorf("parse %q: %w", string(b), err)
	}
	root.baseURL = baseURL
	return root, err
}

func (r Root) ContentPath() string {
	// services = list(map(lambda service: parse_service(service), root.getElementsByTagName('service')))
	// content = [ service for service in services if service['name'] == 'urn:schemas-upnp-org:service:ContentDirectory:1' ][0]
	for _, svc := range r.Device.ServiceList.Service {
		if svc.ServiceType == "urn:schemas-upnp-org:service:ContentDirectory:1" {
			return svc.ControlURL
		}
	}
	return ""
}

func (r Root) post(path, data string) (dl DIDLLite, err error) {
	req, err := http.NewRequest("POST", r.baseURL+path, strings.NewReader(data))
	if err != nil {
		return dl, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("SOAPAction", soapAction)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return dl, err
	}
	defer resp.Body.Close()
	var envelope Envelope
	if err = xml.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return dl, err
	}
	if err = xml.Unmarshal([]byte(envelope.Body.BrowseResponse.Result), &dl); err != nil {
		return dl, fmt.Errorf("unmarshal %q: %w", envelope.Body.BrowseResponse.Result, err)
	}
	return dl, nil
}

// Envelope was generated 2023-05-13 18:40:05 by https://xml-to-go.github.io/ in Ukraine.
type Envelope struct {
	XMLName       xml.Name `xml:"Envelope" json:"envelope,omitempty"`
	Text          string   `xml:",chardata" json:"text,omitempty"`
	S             string   `xml:"s,attr" json:"s,omitempty"`
	EncodingStyle string   `xml:"encodingStyle,attr" json:"encodingstyle,omitempty"`
	Body          struct {
		Text           string `xml:",chardata" json:"text,omitempty"`
		BrowseResponse struct {
			Text           string `xml:",chardata" json:"text,omitempty"`
			U              string `xml:"u,attr" json:"u,omitempty"`
			Result         string `xml:"Result"`
			NumberReturned string `xml:"NumberReturned"`
			TotalMatches   string `xml:"TotalMatches"`
			UpdateID       string `xml:"UpdateID"`
		} `xml:"BrowseResponse" json:"browseresponse,omitempty"`
	} `xml:"Body" json:"body,omitempty"`
}

// DIDLLite was generated 2023-05-13 18:56:01 by https://xml-to-go.github.io/ in Ukraine.
type DIDLLite struct {
	XMLName    xml.Name    `xml:"DIDL-Lite" json:"didl-lite,omitempty"`
	Text       string      `xml:",chardata" json:"text,omitempty"`
	Dc         string      `xml:"dc,attr" json:"dc,omitempty"`
	Upnp       string      `xml:"upnp,attr" json:"upnp,omitempty"`
	Xmlns      string      `xml:"xmlns,attr" json:"xmlns,omitempty"`
	Dlna       string      `xml:"dlna,attr" json:"dlna,omitempty"`
	Containers []Container `xml:"container" json:"container,omitempty"`
	Items      []Item      `xml:"item" json:"item,omitempty"`
}
type Container struct {
	Text        string `xml:",chardata" json:"text,omitempty"`
	ID          string `xml:"id,attr" json:"id,omitempty"`
	ParentID    string `xml:"parentID,attr" json:"parentid,omitempty"`
	Restricted  string `xml:"restricted,attr" json:"restricted,omitempty"`
	Searchable  string `xml:"searchable,attr" json:"searchable,omitempty"`
	ChildCount  string `xml:"childCount,attr" json:"childcount,omitempty"`
	Title       string `xml:"title"`
	Class       string `xml:"class"`
	StorageUsed string `xml:"storageUsed"`
}
type Item struct {
	Text       string `xml:",chardata" json:"text,omitempty"`
	ID         string `xml:"id,attr" json:"id,omitempty"`
	ParentID   string `xml:"parentID,attr" json:"parentid,omitempty"`
	Restricted string `xml:"restricted,attr" json:"restricted,omitempty"`
	Title      string `xml:"title"`
	Class      string `xml:"class"`
	Creator    string `xml:"creator"`
	Date       string `xml:"date"`
	Res        Res    `xml:"res" json:"res,omitempty"`
}
type Res struct {
	URL             string `xml:",chardata" json:"url,omitempty"`
	Size            string `xml:"size,attr" json:"size,omitempty"`
	Duration        string `xml:"duration,attr" json:"duration,omitempty"`
	Bitrate         string `xml:"bitrate,attr" json:"bitrate,omitempty"`
	SampleFrequency string `xml:"sampleFrequency,attr" json:"samplefrequency,omitempty"`
	NrAudioChannels string `xml:"nrAudioChannels,attr" json:"nraudiochannels,omitempty"`
	Resolution      string `xml:"resolution,attr" json:"resolution,omitempty"`
	ProtocolInfo    string `xml:"protocolInfo,attr" json:"protocolinfo,omitempty"`
}

func stripSize(s string) string {
	if before, _, found := strings.Cut(s, "?width="); found {
		return before
	}
	return s
}

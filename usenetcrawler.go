package usenetcrawler

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/tehjojo/newznab"
)

var (
	apiBaseURL = "https://www.usenet-crawler.com/api"
	tvSpecific = apiBaseURL + "?t=tvsearch&rid=%d&cat=%d&season=%d&ep=%d&extended=1"
	comments   = apiBaseURL + "?t=comments&id=%s"
	download   = apiBaseURL + "?t=get&id=%s"

	// CategoryTVHD is the category for high-definition TV shows
	CategoryTVHD = 5040
	// CategoryTVSD is the category for standard-definition TV shows
	CategoryTVSD = 5030
)

// Client is a type for interacting with usenet-crawler
type Client struct {
	apikey string
}

// New returns a new instance of Client
func New(apikey string) Client {
	return Client{
		apikey: apikey,
	}
}

// Search returns NZBs for the given parameters
func (c Client) Search(category int, tvRageID int, season int, episode int) ([]NZB, error) {
	var nzbs []NZB
	log.Debug("usenetcrawler:Client:Search: searching")
	resp, err := getURL(c.withAPIKey(fmt.Sprintf(tvSpecific, tvRageID, category, season, episode)))
	if err != nil {
		return nzbs, err
	}
	var feed newznab.SearchResponse
	err = xml.Unmarshal(resp, &feed)
	if err != nil {
		return nil, err
	}
	log.WithField("num", len(feed.Channel.NZBs)).Info("usenetcrawler:Client:Search: found NZBs")
	for _, gotNZB := range feed.Channel.NZBs {
		log.Debug(remoteNZBJsonString(gotNZB))
		nzb := NZB{
			Title:       gotNZB.Title,
			Description: gotNZB.Description,
			PubDate:     gotNZB.Date.Add(0),
		}

		for _, attr := range gotNZB.Attributes {
			switch attr.Name {
			case "tvairdate":
				if parsedAirDate, err := time.Parse(time.RFC1123Z, attr.Value); err != nil {
					log.Errorf("usenetcrawler:Client:Search: failed to parse date: %v: %v", attr.Value, err)
				} else {
					nzb.AirDate = parsedAirDate
				}
			case "guid":
				nzb.ID = attr.Value
			case "size":
				parsedInt, _ := strconv.ParseInt(attr.Value, 0, 64)
				nzb.Size = parsedInt
			case "grabs":
				parsedInt, _ := strconv.ParseInt(attr.Value, 0, 32)
				nzb.NumGrabs = int(parsedInt)
			case "comments":
				parsedInt, _ := strconv.ParseInt(attr.Value, 0, 32)
				nzb.NumComments = int(parsedInt)
			}
		}
		nzbs = append(nzbs, nzb)
	}
	return nzbs, nil
}

// PopulateComments fills in the Comments for the given NZB
func (c Client) PopulateComments(nzb *NZB) error {
	log.Debug("usenetcrawler:Client:PopulateComments: getting comments")
	data, err := getURL(c.withAPIKey(fmt.Sprintf(comments, nzb.ID)))
	if err != nil {
		return err
	}
	var resp commentResponse
	err = xml.Unmarshal(data, &resp)
	if err != nil {
		return err
	}

	for _, rawComment := range resp.Channel.Comments {
		comment := Comment{
			Title:   rawComment.Title,
			Content: rawComment.Description,
		}
		if parsedPubDate, err := time.Parse(time.RFC1123Z, rawComment.PubDate); err != nil {
			log.WithFields(log.Fields{
				"pub_date": rawComment.PubDate,
				"err":      err,
			}).Error("usenetcrawler:Client:PopulateComments: failed to parse date")
		} else {
			comment.PubDate = parsedPubDate
		}
		nzb.Comments = append(nzb.Comments, comment)
	}
	return nil
}

// DownloadURL returns a URL to download the NZB from
func (c Client) DownloadURL(nzb NZB) string {
	return c.withAPIKey(fmt.Sprintf(download, nzb.ID))
}

// Download returns the bytes of the actual NZB file for the given NZB
func (c Client) Download(nzb NZB) ([]byte, error) {
	return getURL(c.DownloadURL(nzb))
}

func getURL(url string) ([]byte, error) {
	log.WithField("url", url).Debug("usenetcrawler:Client:getURL: getting url")
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	var data []byte
	data, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}

	log.WithField("num_bytes", len(data)).Debug("usenetcrawler:Client:getURL: retrieved")

	return data, nil
}

func (c Client) withAPIKey(url string) string {
	if c.apikey != "" {
		url += "&apikey=" + c.apikey
	}
	return url
}

type commentResponse struct {
	Channel struct {
		Comments []rssComment `xml:"item"`
	} `xml:"channel"`
}

type rssComment struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func remoteNZBJsonString(nzb newznab.NZB) string {
	jsonString, _ := json.MarshalIndent(nzb, "", "  ")
	return string(jsonString)
}

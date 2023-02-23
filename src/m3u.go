package src

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	m3u "threadfin/src/internal/m3u-parser"
)

// Playlisten parsen
func parsePlaylist(filename, fileType string) (channels []interface{}, err error) {

	content, err := readByteFromFile(filename)
	var id = strings.TrimSuffix(getFilenameFromPath(filename), path.Ext(getFilenameFromPath(filename)))
	var playlistName = getProviderParameter(id, fileType, "name")

	if err == nil {

		switch fileType {
		case "m3u":
			channels, err = m3u.MakeInterfaceFromM3U(content)
		case "hdhr":
			channels, err = makeInteraceFromHDHR(content, playlistName, id)
		}

	}

	return
}

// Streams filtern
func filterThisStream(s interface{}) (status bool) {

	status = false
	var stream = s.(map[string]string)
	var regexpYES = `[{]+[^.]+[}]`
	var regexpNO = `!+[{]+[^.]+[}]`

	for _, filter := range Data.Filter {

		if filter.Rule == "" {
			continue
		}

		var group, name, search string
		var exclude, include string
		var match = false

		var streamValues = strings.Replace(stream["_values"], "\r", "", -1)

		if v, ok := stream["group-title"]; ok {
			group = v
		}

		if v, ok := stream["name"]; ok {
			name = v
		}

		// Unerwünschte Streams !{DEU}
		r := regexp.MustCompile(regexpNO)
		val := r.FindStringSubmatch(filter.Rule)

		if len(val) == 1 {

			exclude = val[0][2 : len(val[0])-1]
			filter.Rule = strings.Replace(filter.Rule, " "+val[0], "", -1)
			filter.Rule = strings.Replace(filter.Rule, val[0], "", -1)

		}

		// Muss zusätzlich erfüllt sein {DEU}
		r = regexp.MustCompile(regexpYES)
		val = r.FindStringSubmatch(filter.Rule)

		if len(val) == 1 {

			include = val[0][1 : len(val[0])-1]
			filter.Rule = strings.Replace(filter.Rule, " "+val[0], "", -1)
			filter.Rule = strings.Replace(filter.Rule, val[0], "", -1)

		}

		switch filter.CaseSensitive {

		case false:

			streamValues = strings.ToLower(streamValues)
			filter.Rule = strings.ToLower(filter.Rule)
			exclude = strings.ToLower(exclude)
			include = strings.ToLower(include)
			group = strings.ToLower(group)
			name = strings.ToLower(name)

		}

		switch filter.Type {

		case "group-title":
			search = name

			if group == filter.Rule {
				match = true
			}

		case "custom-filter":
			search = streamValues
			if strings.Contains(search, filter.Rule) {
				match = true
			}
		}

		if match == true {

			if len(exclude) > 0 {
				var status = checkConditions(search, exclude, "exclude")
				if status == false {
					return false
				}
			}

			if len(include) > 0 {
				var status = checkConditions(search, include, "include")
				if status == false {
					return false
				}
			}

			return true

		}

	}

	return false
}

// Bedingungen für den Filter
func checkConditions(streamValues, conditions, coType string) (status bool) {

	switch coType {

	case "exclude":
		status = true

	case "include":
		status = false

	}

	conditions = strings.Replace(conditions, ", ", ",", -1)
	conditions = strings.Replace(conditions, " ,", ",", -1)

	var keys = strings.Split(conditions, ",")

	for _, key := range keys {

		if strings.Contains(streamValues, key) {

			switch coType {

			case "exclude":
				return false

			case "include":
				return true

			}

		}

	}

	return
}

// Threadfin M3U Datei erstellen
func buildM3U(groups []string) (m3u string, err error) {

	var imgc = Data.Cache.Images
	var m3uChannels = make(map[float64]XEPGChannelStruct)
	var channelNumbers []float64

	for _, dxc := range Data.XEPG.Channels {

		var xepgChannel XEPGChannelStruct
		err := json.Unmarshal([]byte(mapToJSON(dxc)), &xepgChannel)
		if err == nil {
			if xepgChannel.XActive == true && !xepgChannel.XHideChannel {

				if len(groups) > 0 {

					if indexOfString(xepgChannel.XGroupTitle, groups) == -1 {
						goto Done
					}

				}

				var channelNumber, err = strconv.ParseFloat(strings.TrimSpace(xepgChannel.XChannelID), 64)

				if err == nil {
					m3uChannels[channelNumber] = xepgChannel
					channelNumbers = append(channelNumbers, channelNumber)
				}

			}

		}

	Done:
	}

	// M3U Inhalt erstellen
	sort.Float64s(channelNumbers)

	var xmltvURL = fmt.Sprintf("%s://%s/xmltv/threadfin.xml", System.ServerProtocol.XML, System.Domain)
	if Settings.ForceHttps && Settings.HttpsThreadfinDomain != "" {
		xmltvURL = fmt.Sprintf("https://%s/xmltv/threadfin.xml", Settings.HttpsThreadfinDomain)
	}
	m3u = fmt.Sprintf(`#EXTM3U url-tvg="%s" x-tvg-url="%s"`+"\n", xmltvURL, xmltvURL)

	for _, channelNumber := range channelNumbers {

		var channel = m3uChannels[channelNumber]

		if Settings.ForceHttps && Settings.HttpsThreadfinDomain != "" {
			u, err := url.Parse(channel.URL)
			if err == nil {
				u.Scheme = "https"
				host_split := strings.Split(u.Host, ":")
				if len(host_split) > 0 {
					u.Host = host_split[0]
				}
				channel.URL = fmt.Sprintf("https://%s:%d%s", u.Host, Settings.HttpsPort, u.Path)
			}
		}

		logo := imgc.Image.GetURL(channel.TvgLogo, Settings.HttpThreadfinDomain, Settings.ForceHttps, Settings.HttpsPort, Settings.HttpsThreadfinDomain)
		var parameter = fmt.Sprintf(`#EXTINF:0 channelID="%s" tvg-chno="%s" tvg-name="%s" tvg-id="%s" tvg-logo="%s" group-title="%s",%s`+"\n", channel.XEPG, channel.XChannelID, channel.XName, channel.XChannelID, logo, channel.XGroupTitle, channel.XName)
		var stream, err = createStreamingURL("M3U", channel.FileM3UID, channel.XChannelID, channel.XName, channel.URL, channel.BackupChannel1URL, channel.BackupChannel2URL, channel.BackupChannel3URL)
		if err == nil {
			m3u = m3u + parameter + stream + "\n"
		}

	}

	if len(groups) == 0 {

		var filename = System.Folder.Data + "threadfin.m3u"
		err = writeByteToFile(filename, []byte(m3u))

	}

	return
}

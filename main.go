package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	logpkg "log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

var (
	log *logpkg.Logger

	API_READY    <-chan time.Time
	RTM          *slack.RTM
	CONFIG_BY_ID map[string]Config

	// flags
	CONFIG_FILE         string
	DEBUG               bool
	DEBUG_SLACK         bool
	DEFAULT_FILE_TTL    int
	DEFAULT_MESSAGE_TTL int
	DRY_RUN             bool
	MAX_RETRIES         int
	SLACK_API_TOKEN_FILE_PATH     string
	SLACK_API_INTERVAL  int
	STATISTICS_HANDLED_MESSAGES	int
	STATISTICS_DELETED_MESSAGES	int
)

func initLog() {
	log = logpkg.New(os.Stdout, "", logpkg.LstdFlags) // |logpkg.LUTCif Ldate or Ltime is set, use UTC rather than the local time zone  // LstdFlags  = Ldate | Ltime // initial values for the standard logger
}

func debug(fmtstr string, args ...interface{}) {
	if !DEBUG {
		return
	}
	log.Printf("D: "+fmtstr, args...)
}

func info(fmtstr string, args ...interface{}) {
	log.Printf("I: "+fmtstr, args...)
}

func errorlog(fmtstr string, args ...interface{}) {
	log.Printf("E: "+fmtstr, args...)
}

func fatal(fmtstr string, args ...interface{}) {
	log.Fatalf("F: "+fmtstr, args...)
}

func jsonString(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

func initApiThrottle() {
	API_READY = time.NewTicker(time.Duration(SLACK_API_INTERVAL) * time.Second).C
}

func initSlackRTMClient() {
	if SLACK_API_TOKEN_FILE_PATH == "" {
		fatal("BLACKHOLE_SLACK_API_TOKEN is not set")
	}
	b, err := ioutil.ReadFile(SLACK_API_TOKEN_FILE_PATH) // just pass the file name
    if err != nil {
        log.Fatal("slack api token file read failed: %s",err);
    }
    apiToken := strings.TrimSpace(string(b)) // convert content to a 'string' and trim whitepsaces

	debug("SLACK_API_TOKEN: %s", apiToken)
	api := slack.New(apiToken)
	slack.OptionLog(log)(api)
	if DEBUG_SLACK {
		slack.OptionDebug(true)(api)
	}
	<-API_READY
	RTM = api.NewRTM()
	go RTM.ManageConnection()

	<-API_READY
	at, err := api.AuthTest()
	if err != nil {
		fatal("AuthTest failed: %v", err)
	}
	debug("Connected to %s as %s", at.Team, at.User)
}

type Config struct {
	Channel    string `json:"channel"`
	MessageTTL int    `json:"message_ttl"`
	FileTTL    int    `json:"file_ttl"`
	OnlyBotMessages    bool    `json:"only_bot_messages"`
}

func initTTL() {
	if CONFIG_FILE == "" {
		fatal("CONFIG_FILE is not specified")
		return
	}
	f, err := os.Open(CONFIG_FILE)
	if err != nil {
		fatal("Open(%s) failed: %v", CONFIG_FILE, err)
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		fatal("ReadAll failed: %v", err)
	}
	cfgs := []Config{}
	err = json.Unmarshal(data, &cfgs)
	if err != nil {
		fatal("Unmarshal(%s) failed: %v", CONFIG_FILE, err)
	}
	debug("Config: %v", cfgs)

	channels, err := getAllChannels(RTM)
	if err != nil {
		fatal("getting the list of channels failed: %v", err)
	}
	channelId := make(map[string]string)
	for _, ch := range channels {
		debug("channelId[%s]: %s", ch.Name, ch.ID)
		channelId[ch.Name] = ch.ID
	}
	for _, cfg := range cfgs {
		debug("CONFIG_BY_ID[%s]: %v", channelId[cfg.Channel], cfg)
		CONFIG_BY_ID[channelId[cfg.Channel]] = cfg
	}

	// direkt übernommen, kann falsch sein

	// for groups
    groups, err := RTM.GetGroups(false)
    if err != nil {
        fatal("GetGroups failed: %v", err)
    }
    groupId := make(map[string]string)
    for _, group := range groups {
        debug("channelId[%s]: %s", group.Name, group.ID)
        groupId[group.Name] = group.ID
    }
    for _, cfg := range cfgs {
        debug("CONFIG_BY_ID[%s]: %v", groupId[cfg.Channel], cfg)
        CONFIG_BY_ID[groupId[cfg.Channel]] = cfg
    }

    // for im channels
    users, err := RTM.GetUsers()
    if err != nil {
        fatal("GetUsers failed: %v", err)
    }
    userName := make(map[string]string)
    for _, user1 := range users {
        debug("userId[%s]: %s", user1.Name, user1.ID)
        userName[user1.ID] = user1.Name
    }
    ims, err := RTM.GetIMChannels()
    if err != nil {
        fatal("GetIMChannels failed: %v", err)
    }
    imChannelId := make(map[string]string)
    for _, im := range ims {
        debug("imChannelId[%s]: %s", im.ID, im.User)
        imChannelId[userName[im.User]] = im.ID
    }

    for _, cfg := range cfgs {
        debug("CONFIG_BY_ID[%s]: %v", imChannelId[cfg.Channel], cfg)
        CONFIG_BY_ID[imChannelId[cfg.Channel]] = cfg
    }

}

func getAllChannels(rtm *slack.RTM) ([]slack.Channel, error) {
	params := &slack.GetConversationsParameters{}
	var channels []slack.Channel
	for cont := true; cont; {
		chs, nextCursor, err := rtm.GetConversations(params)
		if err != nil {
			return nil, fmt.Errorf("GetConversations: %w", err)
		}
		channels = append(channels, chs...)
		params.Cursor = nextCursor
		if nextCursor == "" {
			cont = false
		}
	}
	return channels, nil
}

func unixTime(s string) (time.Time, error) {
	f, err := strconv.ParseFloat(s, 0)
	if err != nil {
		return time.Time{}, err
	}
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1000000000)
	return time.Unix(sec, nsec), nil
}

func deleteMessageSynchronous(ch string, msg *slack.Message, ttl int) {
	ts := msg.Timestamp;
	timeStamp, err := unixTime(ts)
	if err != nil {
		return
	}
	expire :=  timeStamp.Add(time.Duration(ttl) * time.Second)
	debug("Message from %v expire %v", timeStamp, expire)
	if( time.Now().Unix()<expire.Unix() ){
		debug("Message is not yet expired")
		return
	}

	debug("Delete message: %s(%s)", ch, ts)
	STATISTICS_DELETED_MESSAGES++
	if DRY_RUN {
		debug("skip Delete message because of dry run")
		return
	}

	<-API_READY
	_, _, err = RTM.DeleteMessage(ch, ts)
	if err != nil && err.Error() != "message_not_found" {
		errorlog("DeleteMessage(%s, %s) failed: %v", ch, ts, err)
	} else {
		debug("Message deleted: %s(%s)", ch, ts)
		return
	}
	errorlog("Failed to delete message %s(%s) for %d times", ch, ts, MAX_RETRIES)
	return

}


func toBeDeleted(timeStamp string, ttl int) (time.Time, error) {
	ts, err := unixTime(timeStamp)
	if err != nil {
		return ts, err
	}
	return ts.Add(time.Duration(ttl) * time.Second), nil
}

func deleteMessage(ch string, msg *slack.Message, ttl int) {
	ts := msg.Timestamp
	tbd, err := toBeDeleted(ts, ttl)
	if err != nil {
		errorlog("toBeDeleted() for message %s(%s) failed: %v", ch, ts, err)
		return
	}
	info("Message %s(%s) will be deleted at %v", ch, ts, tbd)
	go func() {
		<-time.After(tbd.Sub(time.Now()))
		info("Delete message: %s(%s)", ch, ts)
		if DRY_RUN {
			return
		}

		backoff := time.Duration(1) * time.Second
		for i := 0; i < MAX_RETRIES; i++ {
			<-API_READY
			_, _, err = RTM.DeleteMessage(ch, ts)
			if err != nil && err.Error() != "message_not_found" {
				errorlog("DeleteMessage(%s, %s) failed: %v", ch, ts, err)
			} else {
				info("Message deleted: %s(%s)", ch, ts)
				return
			}
			<-time.After(backoff)
			backoff *= 2
		}
		errorlog("Failed to delete message %s(%s) for %d times", ch, ts, MAX_RETRIES)
	}()
}

func handleMessage(ch string, msg *slack.Message) {
	STATISTICS_HANDLED_MESSAGES++
    debug("Message: %s", jsonString(msg))
	if msg.SubType == "message_deleted" {
		// not a new message
		return
	}

	// todo only delete if "subtype":"bot_message" and config onlyBotMessages = true
	if msg.SubType != "bot_message" && CONFIG_BY_ID[ch].OnlyBotMessages {
		// not a bot message
		debug("skip message: onlyBotMessage Mode active and this is not a bot message")
		return
	}

	cfgttl := CONFIG_BY_ID[ch].MessageTTL
	ttl := DEFAULT_MESSAGE_TTL
	if cfgttl > 0 {
		ttl = cfgttl
	}
	debug("Message %s(%s): cfgttl..%d ttl..%d", ch, msg.Timestamp, cfgttl, ttl)
	if ttl > 0 {
		deleteMessageSynchronous(ch, msg, ttl)
		// deleteMessage(ch, msg, ttl)
	}
}

func handleMessageEvent(msg *slack.MessageEvent) {
	info("MessageEvent: %s(%s)", msg.Channel, msg.Timestamp)
	m := slack.Message(*msg)
	handleMessage(msg.Channel, &m)
}

func deleteFile(file *slack.File, ttl int) {
	ts := file.Timestamp.Time()
	tbd := ts.Add(time.Duration(ttl) * time.Second)
	info("File %s (name='%s' title='%s') created %v (ttl=%d) will be deleted at %v", file.ID, file.Name, file.Title, ts, ttl, tbd)
	go func() {
		<-time.After(tbd.Sub(time.Now()))
		info("Delete File: id=%s name='%s' title='%s'", file.ID, file.Name, file.Title)
		if DRY_RUN {
			return
		}
		backoff := time.Duration(1) * time.Second
		for i := 0; i < MAX_RETRIES; i++ {
			<-API_READY
			err := RTM.DeleteFile(file.ID)
			if err != nil && err.Error() != "file_deleted" {
				errorlog("DeleteFile(%s) failed: %v", file.ID, err)
			} else {
				info("File deleted: %s", file.ID)
				return
			}
			<-time.After(backoff)
			backoff *= 2
		}
		errorlog("Failed to delete file %s for %d times", file.ID, MAX_RETRIES)
	}()
}

func handleFile(file *slack.File) {
	debug("handleFile: %s", jsonString(file))
	if len(file.Channels) == 0 {
		// file from File*Event doesn't have value in Channels field.
		// Re-get if so.
		<-API_READY
		f, _, _, err := RTM.GetFileInfo(file.ID, 0, 1)
		if err != nil {
			fatal("GetFileInfo for %s failed: %v", file.ID, err)
		}
		file = f
	}

	if len(file.Channels) != 1 {
		// file shared to multi channel is not supposed to be deleted
		info("File %s will not be deleted because of channel: %v", file.ID, file.Channels)
		return
	}
	ch := file.Channels[0]
	cfgttl := CONFIG_BY_ID[ch].FileTTL
	ttl := DEFAULT_FILE_TTL
	if cfgttl > 0 {
		ttl = cfgttl
	}
	if ttl > 0 {
		deleteFile(file, ttl)
	}
}

func handleFileCreated(file *slack.FileCreatedEvent) {
	info("File Created: %s", file.File.ID)
	handleFile(&file.File)
}

func handleFileShared(file *slack.FileSharedEvent) {
	info("File Shared: %s", file.File.ID)
	handleFile(&file.File)
}

// blaMarker

func inspectChannelHistory(ch slack.Channel) {
	params := &slack.GetConversationHistoryParameters{
		ChannelID: ch.ID,
	}
	var msgs []slack.Message
	for cont := true; cont; {
		<-API_READY
		res, err := RTM.GetConversationHistory(params)
		if err != nil {
			fatal("GetConversationHistory() for %s failed: %v", ch.ID, err)
		}
		msgs = append(msgs, res.Messages...)
		params.Cursor = res.ResponseMetaData.NextCursor
		if params.Cursor == "" {
			cont = false
		}
	}

	for i := 0; i < len(msgs); i++ {
		handleMessage(ch.ID, &msgs[i])
	}
}

func inspectGroupHistory(group slack.Group) {
	var err error
	debug("inspectHistory group: %s", group.ID)
	h := &slack.History{HasMore: true}
	params := slack.NewHistoryParameters()
	for h.HasMore {
		<-API_READY
		h, err = RTM.GetGroupHistory(group.ID, params)
		if err != nil {
			fatal("GetGroupHistory(%s, %v) failed: %v", group.ID, params, err)
		}
		for i := 0; i < len(h.Messages); i++ {
			handleMessage(group.ID, &h.Messages[i])
		}
		if len(h.Messages) > 0 {
			params.Latest = h.Messages[len(h.Messages)-1].Timestamp
		}
	}
}

func inspectImChannelHistory(im slack.IM) {
	var err error
	debug("inspectHistory imChannel: %s", im.ID)
	h := &slack.History{HasMore: true}
	params := slack.NewHistoryParameters()
	for h.HasMore {
		<-API_READY
		h, err = RTM.GetIMHistory(im.ID, params)
		if err != nil {
			fatal("GetIMHistory(%s, %v) failed: %v", im.ID, params, err)
		}
		for i := 0; i < len(h.Messages); i++ {
			handleMessage(im.ID, &h.Messages[i]) // use userId not channelId
		}
		if len(h.Messages) > 0 {
			params.Latest = h.Messages[len(h.Messages)-1].Timestamp
		}
	}
}

func inspectFiles() {
	params := slack.NewGetFilesParameters()
	debug("NewGetFilesParameters: %v", params)
	for hasMore := true; hasMore; params.Page++ {
		files, paging, err := RTM.GetFiles(params)
		if err != nil {
			fatal("Failed to GetFiles(%v): %v", params, err)
		}
		for i := 0; i < len(files); i++ {
			handleFile(&files[i])
		}

		if paging.Page == paging.Pages {
			hasMore = false
		}
	}
}

func inspectPast() {
	<-API_READY
	channels, err := getAllChannels(RTM)
	if err != nil {
		fatal("getting the list of channels failed: %v", err)
	}
	debug("There are %d channels", len(channels))
	for _, ch := range channels {
		if DEFAULT_MESSAGE_TTL == 0 && CONFIG_BY_ID[ch.ID].MessageTTL == 0 {
			continue
		}
		inspectChannelHistory(ch)
	}

	<-API_READY
	groups, err := RTM.GetGroups(true) // private channels
	if err != nil {
		fatal("GetGroupss() failed: %v", err)
	}
	debug("There are %d groups", len(groups))
	for _, group := range groups {
		if DEFAULT_MESSAGE_TTL == 0 && CONFIG_BY_ID[group.ID].MessageTTL == 0 {
			continue
		}
		inspectGroupHistory(group)
	}

	<-API_READY
	imChannels, err := RTM.GetIMChannels() // direct user im channels
	if err != nil {
		fatal("GetIMChannels() failed: %v", err)
	}
	debug("There are %d imChannels", len(imChannels))
	for _, im := range imChannels {
		debug("imChannel: %s (ID:%s)", im.User, im.ID)
		if DEFAULT_MESSAGE_TTL == 0 && CONFIG_BY_ID[im.ID].MessageTTL == 0 {
			debug("skipped no ttl")
			continue
		}
		inspectImChannelHistory(im)
	}

// get nicht mit bot	inspectFiles()
}


func setFromEnv(f *flag.Flag) {
	envKey := "BLACKHOLE_" + strings.Replace(strings.ToUpper(f.Name), "-", "_", -1)
	envVal := os.Getenv(envKey)
	if envVal != "" {
		err := flag.Set(f.Name, envVal)
		if err != nil {
			fatal("Cannot set flag %s=%s from environment %s: %v", f.Name, envVal, envKey, err)
		}
	}
}

func init() {
	initLog()
	flag.StringVar(&CONFIG_FILE, "config-file", "", "Configuration file")
	flag.BoolVar(&DEBUG, "debug", false, "Debug on")
	flag.BoolVar(&DEBUG_SLACK, "debug-slack", false, "Debug on for Slack")
	flag.IntVar(&DEFAULT_MESSAGE_TTL, "default-message-ttl", 0, "TTL of messages for all channel")
	flag.IntVar(&DEFAULT_FILE_TTL, "default-file-ttl", 0, "TTL of files for all channel")
	flag.BoolVar(&DRY_RUN, "dry-run", false, "Do not delete messages/files")
	flag.IntVar(&MAX_RETRIES, "max-retries", 5, "Maximum number of retries for message/file deletion")
	flag.IntVar(&SLACK_API_INTERVAL, "slack-api-interval", 3, "Interval (sec) for api call")
	flag.StringVar(&SLACK_API_TOKEN_FILE_PATH, "slack-api-token-file", "", "file with Slack API token")
//	flag.VisitAll(setFromEnv)
	CONFIG_BY_ID = make(map[string]Config)
}

func main() {
	flag.Parse()
	initApiThrottle()
	initSlackRTMClient()
	initTTL()

	inspectPast()

	info("%d messages deleted from %d", STATISTICS_DELETED_MESSAGES, STATISTICS_HANDLED_MESSAGES)


// removed all events
}

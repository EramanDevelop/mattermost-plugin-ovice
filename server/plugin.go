package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"net/http"
	"encoding/json"
	"sync"

	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-server/v6/plugin"
	"github.com/mattermost/mattermost-server/v6/model"
)

const (
	botUsername    = "ovice"
	botDisplayName = "oVice"
	botDescription = "A bot account created by the oVice plugin."
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	botUserID string
}

type RequestBody struct {
    ChannelId string `json:"channel_id"`
    Message   string `json:"message"`
}


func (p *Plugin) GetBotUserId() (string, *model.AppError) {
	user, appErr := p.API.GetUserByUsername(botUsername)
	if appErr != nil {
		if appErr.StatusCode != http.StatusNotFound {
			return "", appErr
		}

		botUser, appErr := p.API.CreateBot(&model.Bot{
			Username:    botUsername,
			DisplayName: botDisplayName,
			Description: botDescription,
		})
		if appErr != nil {
			return "", appErr
		}
		return botUser.UserId, nil
	}
	return user.Id, nil
}

// OnActivate ensure the bot account exists
func (p *Plugin) OnActivate() error {
	botUserID, appErr := p.GetBotUserId()
	if appErr != nil {
		return errors.Wrap(appErr, "couldn't get or create bot user")
	}
	p.botUserID = botUserID

	bundlePath, err := p.API.GetBundlePath()
	if err != nil {
		return errors.Wrap(err, "couldn't get bundle path")
	}

	profileImage, err := ioutil.ReadFile(filepath.Join(bundlePath, "assets", "profile.png"))
	if err != nil {
		return errors.Wrap(err, "couldn't read profile image")
	}

	appErr = p.API.SetProfileImage(p.botUserID, profileImage)
	if appErr != nil {
		return errors.Wrap(appErr, "couldn't set profile image")
	}

	return nil
}

// ServeHTTP demonstrates a plugin that handles HTTP requests by greeting the world.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	//parse json
	// https://www.twihike.dev/docs/golang-web/json-request
    var reqBody RequestBody
    if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		p.API.LogError(
			"JSON parse error",
			"err", err.Error(),
		)

        // クライアントが原因のエラーはHTTPステータスコード400を設定
        // サーバが原因のエラーはHTTPステータスコード500を設定
        // エラーメッセージはerr.Error()だけだと分かりづらいため、
        // 原因分類を追加
        var syntaxError *json.SyntaxError
        var unmarshalTypeError *json.UnmarshalTypeError
        switch {
        case errors.As(err, &syntaxError):
            e := fmt.Sprintf("invalid json syntax: %s", err.Error())
            http.Error(w, e, http.StatusBadRequest)
        case errors.As(err, &unmarshalTypeError):
            e := fmt.Sprintf("invalid json field: %s", err.Error())
            http.Error(w, e, http.StatusBadRequest)
        case errors.Is(err, io.EOF):
            e := fmt.Sprintf("request body is empty: %s", err.Error())
            http.Error(w, e, http.StatusBadRequest)
        case errors.Is(err, io.ErrUnexpectedEOF):
            e := fmt.Sprintf("invalid json syntax: %s", err.Error())
            http.Error(w, e, http.StatusBadRequest)
        default:
            http.Error(w, "", http.StatusInternalServerError)
            // エラー内容のログ出力は割愛
        }

		return
    }

	p.processMessage(reqBody.ChannelId, reqBody.Message)

	w.WriteHeader(http.StatusOK)  
	fmt.Fprint(w, "ok")
}


func (p *Plugin) processMessage(channelId string, message string) {
	post := &model.Post{
		Message: message,
		ChannelId: channelId,
		UserId: p.botUserID,
	}

	if _, err := p.API.CreatePost(post); err != nil {
		p.API.LogError(
			"We could not create the response post",
			"user_id", post.UserId,
			"err", err.Error(),
		)
	}
}
// See https://developers.mattermost.com/extend/plugins/server/reference/

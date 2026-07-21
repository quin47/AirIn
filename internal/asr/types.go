package asr

import "context"

type RecognitionResult struct {
	Text      string     `json:"text"`
	Definite  bool       `json:"definite"`
	Utterance *Utterance `json:"utterance,omitempty"`
	SeqNum    int        `json:"seq_num"`
}

type Utterance struct {
	Text      string `json:"text"`
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
	Words     []Word `json:"words,omitempty"`
}

type Word struct {
	Text      string `json:"text"`
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
}

type Client interface {
	Connect(ctx context.Context) error
	SendAudio(pcm []int16) error
	SendEnd() error
	Results() <-chan RecognitionResult
	Errors() <-chan error
	Close() error
}

type Config struct {
	AppID   string `json:"appid"`
	Token   string `json:"token"`
	Cluster string `json:"cluster"`

	UID      string `json:"uid"`
	Language string `json:"language"`

	Format  string `json:"format"`
	Codec   string `json:"codec"`
	Rate    int    `json:"rate"`
	Bits    int    `json:"bits"`
	Channel int    `json:"channel"`

	ShowUtterances bool   `json:"show_utterances"`
	ResultType     string `json:"result_type"`
	NBest          int    `json:"nbest"`
	Workflow       string `json:"workflow"`

	AccessKey string `json:"-"`
	SecretKey string `json:"-"`
}

func DefaultConfig() Config {
	return Config{
		Cluster:  "volcengine_input_edu",
		Language: "zh-CN",

		Format:  "raw",
		Codec:   "raw",
		Rate:    16000,
		Bits:    16,
		Channel: 1,

		ShowUtterances: true,
		ResultType:     "single",
		NBest:          1,
		Workflow:       "audio_in,resample,partition,vad,fe,decode",
	}
}

type fullClientRequest struct {
	App     appInfo     `json:"app"`
	User    userInfo    `json:"user"`
	Audio   audioInfo   `json:"audio"`
	Request requestInfo `json:"request"`
}

type appInfo struct {
	AppID   string `json:"appid"`
	Token   string `json:"token"`
	Cluster string `json:"cluster"`
}

type userInfo struct {
	UID string `json:"uid"`
}

type audioInfo struct {
	Format   string `json:"format"`
	Codec    string `json:"codec"`
	Rate     int    `json:"rate"`
	Bits     int    `json:"bits"`
	Channel  int    `json:"channel"`
	Language string `json:"language"`
}

type requestInfo struct {
	ReqID          string `json:"reqid"`
	Sequence       int    `json:"sequence"`
	ShowUtterances bool   `json:"show_utterances"`
	ResultType     string `json:"result_type"`
	NBest          int    `json:"nbest"`
	Workflow       string `json:"workflow"`
}

type serverResponse struct {
	Sequence   int                `json:"sequence"`
	Code       int                `json:"code"`
	Message    string             `json:"message"`
	Result     []responseUtterance `json:"result,omitempty"`
}

type responseUtterance struct {
	Text     string  `json:"text"`
	Definite bool    `json:"definite"`
	Words    []Word  `json:"words,omitempty"`
	StartTime int    `json:"start_time,omitempty"`
	EndTime   int    `json:"end_time,omitempty"`
}

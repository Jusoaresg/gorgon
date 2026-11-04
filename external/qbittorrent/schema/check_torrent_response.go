package schema

type CheckTorrentResponse struct {
	Name        string  `json:"name"`
	Progress    float32 `json:"progress"`
	State       string  `json:"state"`
	TimeActive  int     `json:"time_active"`
	Completed   int     `json:"completed"`
	CompletedOn int     `json:"completedon"`
	ContentPath string  `json:"content_path"`
	SavePath    string  `json:"save_path"`
	Eta         int     `json:"eta"`
	Hash        string  `json:"hash"`
	NumSeeds    int     `json:"num_seeds"`
	NumLeechs   int     `json:"num_leechs"`
	UpSpeed     int     `json:"upspeed"`
	DlSpeed     int     `json:"dlspeed"`
	Size        int64   `json:"size"`
	AmountLeft  int64   `json:"amount_left"`
	Category    string  `json:"category"`
	AddedOn     int64   `json:"added_on"`
}

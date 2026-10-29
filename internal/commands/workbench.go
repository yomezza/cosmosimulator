package commands

type Command struct {
	Action      string            `json:"action"`
	ID          int64             `json:"id"`
	Ship        int64             `json:"ship"`
	To          int               `json:"to"`
	Star        int               `json:"star"`
	Station     string            `json:"station"`
	Target      string            `json:"target"`
	Resource    string            `json:"resource"`
	Quantity    int               `json:"quantity"`
	Amount      int64             `json:"amount"`
	Price       int64             `json:"price"`
	Region      int               `json:"region"`
	Name        string            `json:"name"`
	Tag         string            `json:"tag"`
	Color       string            `json:"color"`
	Flag        string            `json:"flag"`
	Note        string            `json:"note"`
	Bookmark    bool              `json:"bookmark"`
	Waypoints   []int             `json:"waypoints"`
	Permissions []string          `json:"permissions"`
	IDs         []int64           `json:"ids"`
	Level       string            `json:"level"`
	Message     string            `json:"message"`
	Hours       int               `json:"hours"`
	Collateral  int64             `json:"collateral"`
	FlagDesign  map[string]string `json:"flag_design"`
}

// Package genre defines common genre values used in various Terraonion devices
package genre

// Genre is the genre of a particular game
type Genre int

// This is the list of genres as defined in the MegaSD and SSDS3 firmware
const (
	None Genre = iota
	Shooter
	Action
	Sports
	Misc
	Casino
	Driving
	Platform
	Puzzle
	Boxing
	Wrestling
	Strategy
	Soccer
	Golf
	BeatEmUp
	Baseball
	Mahjong
	Board
	Tennis
	Fighter
	HorseRacing
	Other
)

func (g Genre) String() string {
	strings := map[Genre]string{
		None:        "None",
		Shooter:     "Shooter",
		Action:      "Action",
		Sports:      "Sports",
		Misc:        "Misc",
		Casino:      "Casino",
		Driving:     "Driving",
		Platform:    "Platform",
		Puzzle:      "Puzzle",
		Boxing:      "Boxing",
		Wrestling:   "Wrestling",
		Strategy:    "Strategy",
		Soccer:      "Soccer",
		Golf:        "Golf",
		BeatEmUp:    "Beat'em-Up",
		Baseball:    "Baseball",
		Mahjong:     "Mahjong",
		Board:       "Board",
		Tennis:      "Tennis",
		Fighter:     "Fighter",
		HorseRacing: "Horse Racing",
		Other:       "Other",
	}

	return strings[g]
}

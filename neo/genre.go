package neo

// Genre represents the game genre
type Genre uint32

// These are the currently supported genres and map 1:1 with the NeoSD firmware
const (
	Other Genre = iota
	Action
	BeatEmUp
	Sports
	Driving
	Platformer
	Mahjong
	Shooter
	Quiz
	Fighting
	Puzzle
)

func (g Genre) String() string {
	strings := map[Genre]string{
		Other:      "Other",
		Action:     "Action",
		BeatEmUp:   "BeatEmUp",
		Sports:     "Sports",
		Driving:    "Driving",
		Platformer: "Platformer",
		Mahjong:    "Mahjong",
		Shooter:    "Shooter",
		Quiz:       "Quiz",
		Fighting:   "Fighting",
		Puzzle:     "Puzzle",
	}

	return strings[g]
}

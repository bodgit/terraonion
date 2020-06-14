// +build ignore

package main

import (
	"encoding/xml"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

const (
	featureSlot string = "slot"
)

type softwareLists struct {
	XMLName      xml.Name       `xml:"softwarelists"`
	SoftwareList []softwareList `xml:"softwarelist"`
}

type softwareList struct {
	XMLName  xml.Name   `xml:"softwarelist"`
	Software []software `xml:"software"`
}

type software struct {
	XMLName     xml.Name   `xml:"software"`
	Name        string     `xml:"name,attr"`
	CloneOf     string     `xml:"cloneof,attr"`
	Supported   string     `xml:"supported,attr"`
	Description string     `xml:"description"`
	Year        uint32     `xml:"year"`
	Publisher   string     `xml:"publisher"`
	Feature     []feature  `xml:"part>feature"`
	DataArea    []dataArea `xml:"part>dataarea"`
}

func (s software) Genre() string {
	if extra, ok := mameExtraInformation[s.Name]; ok {
		return extra.genre
	}

	return "Other"
}

func (s software) Reader() string {
	readers := map[string]string{
		"3countb":    "kotm2",
		"alpham2p":   "kotm2p",
		"aof":        "kotm2",
		"bangbead":   "bangbead",
		"burningfp":  "kotm2p",
		"burningfpa": "kotm2p",
		"dragonsh":   "dragonsh",
		"fatfury2":   "kotm2",
		"fightfeva":  "fightfeva",
		"ganryu":     "ganryu",
		"garou":      "garou",
		"garoubl":    "garoubl",
		"garouh":     "garouh",
		"garouha":    "garou",
		"gpilotsp":   "gpilotsp",
		"kf2k3pcb":   "unsupported",
		"kof10th":    "unsupported",
		"kof2000":    "kof2000",
		"kof2000n":   "kof2000n",
		"kof2001":    "kof2001",
		"kof2001h":   "kof2001",
		"kof2003":    "kof2003",
		"kof2003h":   "kof2003h",
		"kof95a":     "kof95a",
		"kof97oro":   "kof97oro",
		"kof99":      "kof99",
		"kof99e":     "kof99",
		"kof99h":     "kof99",
		"kof99k":     "kof99",
		"kof99ka":    "kof99ka",
		"kotm2":      "kotm2",
		"kotm2a":     "kotm2",
		"kotm2p":     "kotm2p",
		"jockeygp":   "jockeygp",
		"jockeygpa":  "jockeygp",
		"lresortp":   "kotm2p",
		"ms4plus":    "ms4plus",
		"ms5plus":    "ms5plus",
		"mslug3":     "mslug3",
		"mslug3a":    "mslug3a",
		"mslug3b6":   "mslug3b6",
		"mslug3h":    "mslug3h",
		"mslug4":     "mslug4",
		"mslug4h":    "mslug4",
		"mslug5":     "mslug5",
		"mslug5h":    "mslug5",
		"mslugx":     "kof95a",
		"nitd":       "nitd",
		"pbobblen":   "kof95a",
		"pbobblenb":  "pbobblenb",
		"pnyaa":      "pnyaa",
		"pnyaaa":     "pnyaa",
		"preisle2":   "preisle2",
		"rotd":       "rotd",
		"rotdh":      "rotd",
		"s1945p":     "s1945p",
		"samsho3":    "kof95a",
		"sengoku2":   "kotm2",
		"sengoku3":   "sengoku3",
		"sengoku3a":  "sengoku3",
		"ssideki":    "viewpoin",
		"svc":        "svc",
		"viewpoin":   "viewpoin",
		"viewpoinp":  "gpilotsp",
		"wh1":        "kotm2",
		"wh1h":       "kotm2",
		"wh1ha":      "kotm2",
		"zupapa":     "zupapa",
	}

	if reader, ok := readers[s.Name]; ok {
		return reader
	}

	return "common"
}

func (s software) FindDataArea(area string) *dataArea {
	if area == "audiocpu" {
		// Try and find an audiocrypt area first, both don't ever appear
		if da := s.FindDataArea("audiocrypt"); da != nil {
			return da
		}
	}
	for _, da := range s.DataArea {
		if da.Name == area {
			return &da
		}
	}
	return nil
}

func (s software) Screenshot() int {
	if extra, ok := mameExtraInformation[s.Name]; ok {
		return extra.screenshot
	}

	return 0
}

func (s software) IsSupportedSlot() bool {
	for _, f := range s.Feature {
		if f.Name == featureSlot {
			switch f.Value {
			case "boot_garoubl", "boot_kf10th", "boot_kof97oro", "boot_ms5plus", "boot_mslug3b6":
				fallthrough
			case "rom_fatfur2", "rom_mslugx":
				fallthrough
			case "cmc42_bangbead", "cmc42_ganryu", "cmc42_kof99k", "cmc42_mslug3h", "cmc42_nitd", "cmc42_preisle2", "cmc42_s1945p", "cmc42_sengoku3", "cmc42_zupapa":
				fallthrough
			case "cmc50_kof2000n", "cmc50_kof2001", "cmc50_jockeygp":
				fallthrough
			case "pcm2_ms4p", "pcm2_mslug4", "pcm2_pnyaa", "pcm2_rotd":
				fallthrough
			case "pvc_kf2k3", "pvc_kf2k3h", "pvc_mslug5", "pvc_svc":
				fallthrough
			case "sma_garou", "sma_garouh", "sma_kof2k", "sma_kof99", "sma_mslug3", "sma_mslug3a":
				return true
			default:
				return false
			}
		}
	}
	return true
}

type feature struct {
	XMLName xml.Name `xml:"feature"`
	Name    string   `xml:"name,attr"`
	Value   string   `xml:"value,attr"`
}

type size uint64

func (v *size) UnmarshalXMLAttr(attr xml.Attr) error {
	i, err := strconv.ParseUint(attr.Value, 0, 64)
	if err != nil {
		return err
	}
	*v = size(i)
	return nil
}

type dataArea struct {
	XMLName xml.Name `xml:"dataarea"`
	Name    string   `xml:"name,attr"`
	Size    size     `xml:"size,attr"`
	ROM     []rom    `xml:"rom"`
}

func (d dataArea) IsEmpty() bool {
	count := 0
	for _, r := range d.ROM {
		if r.Status != "nodump" {
			count++
		}
	}
	return count == 0
}

type rom struct {
	XMLName xml.Name `xml:"rom"`
	Name    string   `xml:"name,attr"`
	Size    size     `xml:"size,attr"`
	CRC     string   `xml:"crc,attr"`
	Status  string   `xml:"status,attr"`
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	dir, err := os.Open(cwd)
	if err != nil {
		log.Fatal(err)
	}

	names, err := dir.Readdirnames(0)
	if err != nil {
		log.Fatal(err)
	}

	var games softwareLists

	for _, name := range names {
		if filepath.Ext(name) == ".xml" {
			b, err := ioutil.ReadFile(name)
			if err != nil {
				log.Fatal(err)
			}

			if err := xml.Unmarshal(b, &games); err != nil {
				log.Fatal(err)
			}
		}
	}

	f, err := os.Create("games.go")
	if err != nil {
		log.Fatal(err)
	}

	if err := tmpl.Execute(f, games); err != nil {
		log.Fatal(err)
	}
}

var funcs = template.FuncMap{
	"areas": func() []string {
		return []string{"maincpu", "fixed", "audiocpu", "ymsnd", "ymsnd.deltat", "sprites"}
	},
	"bytes": func(value string) string {
		b := []string{}
		for i := 0; i < len(value); i += 2 {
			b = append(b, "0x"+value[i:i+2])
		}
		return strings.ToLower(strings.Join(b, ", "))
	},
}

var tmpl = template.Must(template.New("").Funcs(funcs).Parse(`// Code generated by go generate; DO NOT EDIT.

package neo

var mameGames = map[string]struct {
	mameGame
	reader       gameReader
	name         string
	year         uint32
	manufacturer string
	genre        Genre
	screenshot   uint32
}{
{{- range .SoftwareList }}
{{- range .Software }}
{{- if and (ne .Supported "no") .IsSupportedSlot }}
	"{{ .Name }}": {
		mameGame{
			{{ if .CloneOf }}"{{ .CloneOf }}"{{ else }}""{{ end }},
			[...]mameArea{
{{- $game := . }}
{{- range areas }}
{{- with $game.FindDataArea . }}
				{
					{{ .Size }},
{{- if .IsEmpty }}
					[]mameROM{},
{{- else }}
					[]mameROM{
{{- range .ROM }}
{{- if and (ne .Status "nodump") (ne .Name "") }}
						{
							"{{ .Name }}",
							{{ .Size }},
							[]byte{{"{"}}{{bytes .CRC}}{{"}"}},
						},
{{- end }}
{{- end }}
					},
{{- end }}
				},
{{- else }}
				{
					0,
					[]mameROM{},
				},
{{- end }}
{{- end }}
			},
		},
		{{ .Reader }}{},
		"{{ .Description }}",
		{{ .Year }},
		"{{ .Publisher }}",
		{{ .Genre }},
		{{ .Screenshot }},
	},
{{- end }}
{{- end }}
{{- end }}
}
`))

// mameExtraInformation augments the external MAME database with the
// screenshot ID that is internal to the NeoSD firmware and provided by
// `order.txt` found in the Neobuilder tool distribution. The genre codes
// are internal to the same tool but don't have an external list.
var mameExtraInformation = map[string]struct {
	screenshot int
	genre      string
}{
	"alpham2":    {1, "Shooter"},
	"alpham2p":   {1, "Shooter"},
	"androdun":   {2, "Shooter"},
	"aodk":       {3, "Fighting"},
	"aof":        {4, "Fighting"},
	"aof2":       {5, "Fighting"},
	"aof2a":      {5, "Fighting"},
	"aof3":       {6, "Fighting"},
	"aof3k":      {6, "Fighting"},
	"b2b":        {7, "Action"},
	"bakatono":   {8, "Mahjong"},
	"bangbead":   {9, "Sports"},
	"bjourney":   {10, "Platformer"},
	"blazstar":   {11, "Shooter"},
	"breakers":   {12, "Fighting"},
	"breakrev":   {13, "Fighting"},
	"bstars":     {14, "Sports"},
	"bstarsh":    {14, "Sports"},
	"bstars2":    {15, "Sports"},
	"burningf":   {16, "BeatEmUp"},
	"burningfh":  {16, "BeatEmUp"},
	"burningfp":  {16, "BeatEmUp"},
	"burningfpa": {16, "BeatEmUp"},
	"crswd2bl":   {17, "Action"},
	"crsword":    {18, "Action"},
	"ct2k3sp":    {19, "Fighting"},
	"ct2k3sa":    {19, "Fighting"},
	"cthd2003":   {20, "Fighting"},
	"ctomaday":   {21, "Shooter"},
	"cyberlip":   {22, "Action"},
	"diggerma":   {23, "Puzzle"},
	"doubledr":   {24, "Fighting"},
	"dragonsh":   {25, "Fighting"},
	"eightman":   {26, "Action"},
	"fatfursp":   {27, "Fighting"},
	"fatfurspa":  {27, "Fighting"},
	"fatfury1":   {28, "Fighting"},
	"fatfury2":   {29, "Fighting"},
	"fatfury3":   {30, "Fighting"},
	"fbfrenzy":   {31, "Sports"},
	"fightfev":   {32, "Fighting"},
	"fightfeva":  {32, "Fighting"},
	"flipshot":   {33, "Sports"},
	"froman2b":   {34, "Mahjong"},
	"fswords":    {35, "Fighting"},
	"galaxyfg":   {36, "Fighting"},
	"ganryu":     {37, "Action"},
	"garou":      {38, "Fighting"},
	"garouh":     {38, "Fighting"},
	"garouha":    {38, "Fighting"},
	"garoup":     {38, "Fighting"},
	"garoubl":    {38, "Fighting"},
	"ghostlop":   {39, "Puzzle"},
	"goalx3":     {40, "Sports"},
	"gowcaizr":   {41, "Fighting"},
	"gpilots":    {42, "Shooter"},
	"gpilotsh":   {43, "Shooter"},
	"gururin":    {44, "Puzzle"},
	"ironclad":   {45, "Shooter"},
	"ironclado":  {45, "Shooter"},
	"irrmaze":    {46, "Other"}, // TODO
	"janshin":    {47, "Mahjong"},
	"jockeygp":   {48, "Sports"},
	"jockeygpa":  {48, "Sports"},
	"joyjoy":     {49, "Puzzle"},
	"kabukikl":   {50, "Fighting"},
	"karnovr":    {51, "Fighting"},
	"kf10thep":   {52, "Fighting"},
	"kf2k2mp":    {53, "Fighting"},
	"kf2k2mp2":   {54, "Fighting"},
	"kf2k2pla":   {55, "Fighting"},
	"kf2k2pls":   {56, "Fighting"},
	"kf2k3bl":    {57, "Fighting"},
	"kf2k3bla":   {57, "Fighting"},
	"kf2k3pl":    {58, "Fighting"},
	"kf2k3upl":   {58, "Fighting"},
	"kf2k5uni":   {59, "Fighting"},
	"kizuna":     {60, "Fighting"},
	"kof10th":    {62, "Fighting"}, // XXX Unsupported
	"kof2000":    {63, "Fighting"},
	"kof2000n":   {63, "Fighting"},
	"kof2001":    {64, "Fighting"},
	"kof2001h":   {64, "Fighting"},
	"kof2002":    {65, "Fighting"},
	"kof2002b":   {65, "Fighting"},
	"kof2003":    {66, "Fighting"},
	"kof2003h":   {66, "Fighting"},
	"kf2k3pcb":   {66, "Fighting"}, // XXX Unsupported
	"kof2k4se":   {67, "Fighting"},
	"kof94":      {68, "Fighting"},
	"kof95":      {69, "Fighting"},
	"kof95a":     {69, "Fighting"},
	"kof95h":     {69, "Fighting"},
	"kof96":      {70, "Fighting"},
	"kof96h":     {70, "Fighting"},
	"kof97":      {71, "Fighting"},
	"kof97h":     {71, "Fighting"},
	"kof97k":     {71, "Fighting"},
	"kof97oro":   {72, "Fighting"},
	"kof97pls":   {73, "Fighting"},
	"kof98":      {74, "Fighting"},
	"kof98a":     {74, "Fighting"},
	"kof98h":     {74, "Fighting"},
	"kof98k":     {74, "Fighting"},
	"kof98ka":    {74, "Fighting"},
	"kof99":      {75, "Fighting"},
	"kof99e":     {75, "Fighting"},
	"kof99h":     {75, "Fighting"},
	"kof99k":     {75, "Fighting"},
	"kof99ka":    {75, "Fighting"}, // FIXME Not in order.txt
	"kof99p":     {75, "Fighting"},
	"kog":        {76, "Fighting"},
	"kotm":       {77, "Fighting"},
	"kotm2":      {78, "Fighting"},
	"kotm2a":     {78, "Fighting"},
	"kotm2p":     {78, "Fighting"},
	"kotmh":      {79, "Fighting"},
	"lans2004":   {80, "Action"},
	"lastblad":   {81, "Fighting"},
	"lastbladh":  {81, "Fighting"},
	"lastbld2":   {82, "Fighting"},
	"lastsold":   {84, "Fighting"},
	"lbowling":   {85, "Sports"},
	"legendos":   {86, "BeatEmUp"},
	"lresort":    {87, "Shooter"},
	"lresortp":   {87, "Shooter"},
	"magdrop2":   {88, "Puzzle"},
	"magdrop3":   {89, "Puzzle"},
	"maglord":    {90, "Platformer"},
	"maglordh":   {90, "Platformer"},
	"mahretsu":   {91, "Mahjong"},
	"marukodq":   {92, "Quiz"},
	"matrim":     {93, "Fighting"},
	"matrimbl":   {93, "Fighting"},
	"miexchng":   {94, "Puzzle"},
	"minasan":    {95, "Other"},
	"moshougi":   {96, "Other"},
	"ms4plus":    {97, "Action"},
	"ms5plus":    {98, "Action"},
	"mslug":      {99, "Action"},
	"mslug2":     {100, "Action"},
	"mslug2t":    {100, "Action"}, // TODO
	"mslug3":     {101, "Action"},
	"mslug3a":    {101, "Action"},
	"mslug3h":    {101, "Action"},
	"mslug3b6":   {102, "Action"},
	"mslug4":     {103, "Action"},
	"mslug4h":    {103, "Action"},
	"mslug5":     {104, "Action"},
	"mslug5h":    {104, "Action"},
	"ms5pcb":     {104, "Action"}, // TODO
	"mslugx":     {105, "Action"},
	"mutnat":     {106, "BeatEmUp"},
	"nam1975":    {107, "Action"},
	"ncombat":    {108, "BeatEmUp"},
	"ncombath":   {108, "BeatEmUp"},
	"ncommand":   {109, "Shooter"},
	"neobombe":   {110, "Puzzle"},
	"neocup98":   {111, "Sports"},
	"neodrift":   {112, "Driving"},
	"neomrdo":    {113, "Action"},
	"ninjamas":   {114, "Fighting"},
	"nitd":       {115, "Action"},
	"nitdbl":     {116, "Action"},
	"overtop":    {117, "Driving"},
	"panicbom":   {118, "Puzzle"},
	"pbobbl2n":   {119, "Puzzle"},
	"pbobblen":   {120, "Puzzle"},
	"pbobblenb":  {120, "Puzzle"},
	"pgoal":      {121, "Sports"},
	"pnyaa":      {122, "Other"}, // TODO
	"pnyaaa":     {122, "Other"}, // TODO
	"popbounc":   {123, "Puzzle"},
	"preisle2":   {124, "Shooter"},
	"pspikes2":   {125, "Sports"},
	"pulstar":    {126, "Shooter"},
	"puzzldpr":   {127, "Puzzle"},
	"puzzledp":   {127, "Puzzle"},
	"quizdai2":   {128, "Quiz"},
	"quizdais":   {129, "Quiz"},
	"quizdaisk":  {129, "Quiz"},
	"quizkof":    {130, "Quiz"},
	"quizkofk":   {130, "Quiz"},
	"ragnagrd":   {131, "Fighting"},
	"rbff1":      {132, "Fighting"},
	"rbff1a":     {132, "Fighting"},
	"rbff1k":     {132, "Fighting"},
	"rbff2":      {133, "Fighting"},
	"rbff2h":     {133, "Fighting"},
	"rbff2k":     {133, "Fighting"},
	"rbffspec":   {134, "Fighting"},
	"rbffspeck":  {134, "Fighting"},
	"ridhero":    {135, "Driving"},
	"ridheroh":   {135, "Driving"},
	"roboarmy":   {136, "BeatEmUp"},
	"rotd":       {137, "Fighting"},
	"rotdh":      {137, "Fighting"},
	"s1945p":     {138, "Shooter"},
	"samsh5sp":   {139, "Fighting"},
	"samsh5sph":  {139, "Fighting"},
	"samsh5spho": {139, "Fighting"},
	"samsho":     {140, "Fighting"},
	"samshoh":    {140, "Fighting"},
	"samsho2":    {141, "Fighting"},
	"samsho2k":   {141, "Fighting"},
	"samsho2ka":  {141, "Fighting"}, // FIXME Not in order.txt
	"samsho3":    {142, "Fighting"},
	"samsho3h":   {142, "Fighting"},
	"samsho3k":   {142, "Fighting"},
	"samsho4":    {143, "Fighting"},
	"samsho4k":   {143, "Fighting"},
	"samsho5":    {144, "Fighting"},
	"samsho5h":   {144, "Fighting"},
	"samsho5b":   {144, "Fighting"},
	"savagere":   {145, "Fighting"},
	"sbp":        {146, "Puzzle"},
	"sdodgeb":    {147, "Sports"},
	"sengoku":    {148, "BeatEmUp"},
	"sengokuh":   {148, "BeatEmUp"},
	"sengoku2":   {149, "BeatEmUp"},
	"sengoku3":   {150, "BeatEmUp"},
	"shocktr2":   {151, "Action"},
	"shocktro":   {152, "Action"}, // TODO
	"shocktroa":  {152, "Action"},
	"socbrawl":   {153, "Sports"},
	"socbrawlh":  {153, "Sports"},
	"sonicwi2":   {154, "Shooter"},
	"sonicwi3":   {155, "Shooter"},
	"spinmast":   {156, "Platformer"},
	"ssideki":    {157, "Sports"},
	"ssideki2":   {158, "Sports"},
	"ssideki3":   {159, "Sports"},
	"ssideki4":   {160, "Sports"},
	"stakwin":    {161, "Sports"},
	"stakwin2":   {162, "Sports"},
	"strhoop":    {163, "Sports"},
	"superspy":   {164, "BeatEmUp"},
	"svc":        {165, "Fighting"},
	"svcboot":    {165, "Fighting"},
	"svcplus":    {166, "Fighting"},
	"svcplusa":   {166, "Fighting"},
	"svcsplus":   {167, "Fighting"},
	"svcpcb":     {165, "Fighting"}, // TODO
	"svcpcba":    {165, "Fighting"}, // TODO
	"tophuntr":   {168, "Platformer"},
	"tophuntrh":  {168, "Platformer"},
	"tpgolf":     {169, "Sports"},
	"trally":     {170, "Driving"},
	"turfmast":   {171, "Sports"},
	"twinspri":   {172, "Puzzle"},
	"tws96":      {173, "Other"}, // TODO
	"viewpoin":   {174, "Shooter"},
	"vliner":     {175, "Other"}, // TODO
	"vlinero":    {175, "Other"}, // TODO
	"wakuwak7":   {176, "Fighting"},
	"wh1":        {177, "Fighting"},
	"wh1h":       {177, "Fighting"},
	"wh1ha":      {177, "Fighting"},
	"wh2":        {178, "Fighting"},
	"wh2j":       {179, "Fighting"},
	"whp":        {180, "Fighting"},
	"wjammers":   {181, "Sports"},
	"zedblade":   {182, "Shooter"},
	"zintrckb":   {183, "Puzzle"},
	"zupapa":     {184, "Action"},
	"2020bb":     {185, "Sports"},
	"2020bba":    {185, "Sports"},
	"2020bbh":    {185, "Sports"},
	"3countb":    {186, "Fighting"},
}

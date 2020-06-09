package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bodgit/terraonion/neo"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
)

func init() {
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"V"},
		Usage:   "print the version",
	}
}

func romToString(r int) string {
	switch r {
	case neo.P:
		return "P"
	case neo.S:
		return "S"
	case neo.M:
		return "M"
	case neo.V1:
		return "V1"
	case neo.V2:
		return "V2"
	case neo.C:
		return "C"
	default:
		return strconv.Itoa(r)
	}
}

func info(c *cli.Context) error {
	if c.NArg() < 1 {
		cli.ShowCommandHelpAndExit(c, c.Command.FullName(), 1)
	}

	b, err := ioutil.ReadFile(c.Args().First())
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	f := new(neo.File)
	if err := f.UnmarshalBinary(b); err != nil {
		return cli.NewExitError(err, 1)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetBorder(false)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetTablePadding(" ")
	table.SetNoWhiteSpace(true)

	table.Append([]string{"Name:", f.Name})
	table.Append([]string{"Manufacturer:", f.Manufacturer})
	table.Append([]string{"Year:", strconv.FormatUint(uint64(f.Year), 10)})
	table.Append([]string{"Genre:", f.Genre.String()})
	table.Append([]string{"Screenshot:", strconv.FormatUint(uint64(f.Screenshot), 10)})
	table.Append([]string{"NGH:", fmt.Sprintf("0x%x", f.NGH)})

	table.Render()

	if c.Bool("verbose") {
		fmt.Println()

		table := tablewriter.NewWriter(os.Stdout)
		table.SetBorder(false)
		table.SetCenterSeparator("")
		table.SetColumnSeparator("")

		table.SetHeader([]string{"ROM", "Size", "SHA1"})

		for i := 0; i < neo.Areas; i++ {
			if f.Size[i] > 0 {
				h := sha1.New()

				if _, err := io.Copy(h, bytes.NewBuffer(f.ROM[i])); err != nil {
					return cli.NewExitError(err, 1)
				}

				table.Append([]string{romToString(i), strconv.FormatUint(uint64(f.Size[i]), 10), fmt.Sprintf("%x", h.Sum(nil))})
			} else {
				table.Append([]string{romToString(i), "0", "-"})
			}
		}

		table.Render()
	}

	return nil
}

func convert(c *cli.Context) error {
	if c.NArg() < 1 {
		cli.ShowCommandHelpAndExit(c, c.Command.FullName(), 1)
	}

	path := c.Args().First()

	n, err := neo.NewFile(path)
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	if c.IsSet("name") {
		n.Name = c.String("name")
	}

	if c.IsSet("manufacturer") {
		n.Manufacturer = c.String("manufacturer")
	}

	if c.IsSet("year") {
		n.Year = uint32(c.Uint("year"))
	}

	if c.IsSet("genre") {
		n.Genre = neo.Genre(c.Uint("genre"))
	}

	if c.IsSet("screenshot") {
		n.Screenshot = uint32(c.Uint("screenshot"))
	}

	f, err := os.Create(filepath.Join(c.String("directory"), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+neo.Extension))
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	b, err := n.MarshalBinary()
	if err != nil {
		return cli.NewExitError(err, 1)
	}

	if _, err := f.Write(b); err != nil {
		return cli.NewExitError(err, 1)
	}

	return nil
}

func main() {
	app := cli.NewApp()

	app.Name = "neosd"
	app.Usage = "Terraonion NeoSD management utility"
	app.Version = "1.0.0"

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	app.Commands = []*cli.Command{
		{
			Name:        "info",
			Usage:       "Info on a " + neo.Extension + " file",
			Description: "",
			Action:      info,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "verbose",
					Aliases: []string{"v"},
					Usage:   "increase verbosity",
				},
			},
		},
		{
			Name:        "convert",
			Usage:       "Create a " + neo.Extension + " file from an existing set of ROM images",
			Description: "",
			Action:      convert,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "directory",
					Aliases: []string{"d"},
					Usage:   "output directory",
					Value:   cwd,
				},
				&cli.StringFlag{
					Name:  "name",
					Usage: "override name with `NAME`",
				},
				&cli.StringFlag{
					Name:  "manufacturer",
					Usage: "override manufacturer with `MANUFACTURER`",
				},
				&cli.UintFlag{
					Name:  "year",
					Usage: "override year with `YEAR`",
				},
				&cli.UintFlag{
					Name:  "genre",
					Usage: "override genre with `GENRE`",
				},
				&cli.UintFlag{
					Name:  "screenshot",
					Usage: "override screenshot with `SCREENSHOT`",
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

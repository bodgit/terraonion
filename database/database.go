/*
Package database uses SQLite to store game screenshots and CRC as an
alternative to the XML file and separate PNG files used by the original C#
based utilities.
*/
package database

import (
	"bytes"
	"crypto/sha1"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/terraonion/genre"

	// Database driver
	_ "github.com/mattn/go-sqlite3"
)

// Database holds the SQLite database handle and image encoding function
type Database struct {
	db     *sql.DB
	encode func(io.Writer, image.Image) error
}

// NewDatabase opens an existing database or returns a new empty one. An image
// encoding function should also be provided to encode screenshots for the
// target device
func NewDatabase(file string, encode func(io.Writer, image.Image) error) (*Database, error) {
	if file == "" {
		return nil, errors.New("no file")
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on", file))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)

	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS screenshot (id INTEGER PRIMARY KEY NOT NULL, sha1 TEXT NOT NULL UNIQUE, data BLOB NOT NULL)"); err != nil {
		return nil, err
	}

	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS game (id INTEGER PRIMARY KEY NOT NULL, name STRING NOT NULL UNIQUE, screenshot_id INTEGER, genre INTEGER, year INTEGER, FOREIGN KEY(screenshot_id) REFERENCES screenshot(id))"); err != nil {
		return nil, err
	}

	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS checksum(game_id INTEGER NOT NULL, crc TEXT NOT NULL UNIQUE, FOREIGN KEY(game_id) REFERENCES game(id))"); err != nil {
		return nil, err
	}

	return &Database{
		db:     db,
		encode: encode,
	}, nil
}

type xmlGameDB struct {
	XMLName   xml.Name      `xml:"GameDB"`
	Games     []xmlGame     `xml:"Game"`
	Checksums []xmlChecksum `xml:"GameCk"`
	Genres    []xmlGenre    `xml:"Genre"`
}

type xmlGame struct {
	XMLName    xml.Name `xml:"Game"`
	ID         int      `xml:"ID"`
	Name       string   `xml:"Name"`
	Year       int64    `xml:"Year"`
	Genre      int64    `xml:"Genre"`
	Screenshot string   `xml:"Screenshot"`
}

type xmlChecksum struct {
	XMLName  xml.Name `xml:"GameCk"`
	Checksum string   `xml:"Checksum"`
	GameID   int      `xml:"GameID"`
}

type xmlGenre struct {
	XMLName xml.Name `xml:"Genre"`
	Genre   int      `xml:"Genre"`
	Name    string   `xml:"Name"`
}

// ImportXML wipes an existing data and imports the entries from an XML file
func (db *Database) ImportXML(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}

	var xmlDB xmlGameDB
	if err := xml.Unmarshal(b, &xmlDB); err != nil {
		return err
	}

	if _, err = db.db.Exec("DELETE FROM checksum"); err != nil {
		return err
	}

	if _, err = db.db.Exec("DELETE FROM game"); err != nil {
		return err
	}

	if _, err = db.db.Exec("DELETE FROM screenshot"); err != nil {
		return err
	}

	for _, g := range xmlDB.Games {
		var year sql.NullInt64
		if g.Year != 0 {
			year.Int64 = g.Year
			year.Valid = true
		}

		var genre sql.NullInt64
		if g.Genre != 0 {
			genre.Int64 = g.Genre
			genre.Valid = true
		}

		var screenshot sql.NullInt64
		if g.Screenshot != "" {
			log.Println(filepath.Join(filepath.Dir(f.Name()), filepath.Clean(strings.ReplaceAll(g.Screenshot, "\\", string(os.PathSeparator)))))
			screenshot.Int64, err = db.addScreenshot(filepath.Join(filepath.Dir(f.Name()), filepath.Clean(strings.ReplaceAll(g.Screenshot, "\\", string(os.PathSeparator)))))
			if err != nil {
				return err
			}
			screenshot.Valid = true
		}

		game, err := db.addGame(g.Name, year, genre, screenshot)
		if err != nil {
			return err
		}

		for _, c := range xmlDB.Checksums {
			if g.ID == c.GameID {
				db.addChecksum(game, fmt.Sprintf("%08s", strings.ToUpper(c.Checksum)))
			}
		}
	}

	return nil
}

// Close closes the database rendering it unusable
func (db *Database) Close() error {
	return db.db.Close()
}

func (db *Database) addScreenshot(file string) (int64, error) {
	f, err := os.Open(file)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	h := sha1.New()
	m, _, err := image.Decode(io.TeeReader(f, h))
	if err != nil {
		return 0, err
	}
	sha := fmt.Sprintf("%X", h.Sum(nil))

	var id int64
	switch err := db.db.QueryRow("SELECT id FROM screenshot WHERE sha1 = ?", sha).Scan(&id); err {
	case sql.ErrNoRows:
		b := new(bytes.Buffer)
		if err := db.encode(b, m); err != nil {
			return 0, err
		}
		result, err := db.db.Exec("INSERT INTO screenshot (sha1, data) VALUES (?, ?)", sha, b.Bytes())
		if err != nil {
			return 0, err
		}
		return result.LastInsertId()
	case nil:
		return id, nil
	default:
		return 0, err
	}
}

func (db *Database) addGame(name string, year, genre, screenshot sql.NullInt64) (int64, error) {
	var id int64
	switch err := db.db.QueryRow("SELECT id FROM game WHERE name = ? AND year = ? AND genre = ? AND screenshot_id = ?", name, year, genre, screenshot).Scan(&id); err {
	case sql.ErrNoRows:
		result, err := db.db.Exec("INSERT INTO game (name, year, genre, screenshot_id) VALUES (?, ?, ?, ?)", name, year, genre, screenshot)
		if err != nil {
			return 0, err
		}
		return result.LastInsertId()
	case nil:
		return id, nil
	default:
		return 0, err
	}
}

func (db *Database) addChecksum(game int64, crc string) error {
	if _, err := db.db.Exec("INSERT OR REPLACE INTO checksum (game_id, crc) VALUES (?, ?)", game, crc); err != nil {
		return err
	}
	return nil
}

// FindScreenshotByCRC searchs the database for a game matching the CRC and
// returns the screenshot, year and genre
func (db *Database) FindScreenshotByCRC(crc string) ([]byte, int, genre.Genre, error) {
	var year, genreValue sql.NullInt64
	var data []byte
	switch err := db.db.QueryRow("SELECT g.year, g.genre, s.data FROM checksum AS c JOIN game AS g ON c.game_id = g.id LEFT JOIN screenshot AS s ON g.screenshot_id = s.id WHERE c.crc = ?", crc).Scan(&year, &genreValue, &data); err {
	case sql.ErrNoRows:
		return nil, 0, genre.None, nil
	case nil:
		y := 0
		if year.Valid {
			y = int(year.Int64)
		}

		g := genre.None
		if genreValue.Valid {
			g = genre.Genre(genreValue.Int64)
		}

		return data, y, g, nil
	default:
		return nil, 0, genre.None, err
	}
}

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/oschwald/maxminddb-golang"
	"github.com/rs/xid"

	_ "github.com/lib/pq"
)

type City struct {
	City struct {
		GeoNameID uint              `maxminddb:"geoname_id"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Country struct {
		GeoNameID         uint              `maxminddb:"geoname_id"`
		IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
		IsoCode           string            `maxminddb:"iso_code"`
		Names             map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	Subdivisions []struct {
		GeoNameID uint              `maxminddb:"geoname_id"`
		IsoCode   string            `maxminddb:"iso_code"`
		Names     map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
}

type visitorData struct {
	ID			int
	FingerPrint	string
	Browser		string
	City		string
	State		string
	Country		string
	UserID		string
	LastUpdate	string
}

type User struct {
	UserID string `json:"id,omitempty"`
}

var db *sql.DB
var geoIpDb *maxminddb.Reader

const (
    dbhost = "DBHOST"
    dbport = "DBPORT"
    dbuser = "DBUSER"
    dbpass = "DBPASS"
    dbname = "DBNAME"
)

func GetUserId(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	fingerPrint := params["fingerPrint"]
	
	browser := getBrowser(params["userAgent"])

	city := ""
	state := ""
	country := ""
	ipInfo := getIpInfo(params["ip"])
	if ipInfo != nil {
		city = ipInfo.City.Names["en"]
		state = ipInfo.Subdivisions[0].Names["en"]
		country = ipInfo.Country.Names["en"]
	}

	visitorData := visitorData{FingerPrint: fingerPrint, Browser: browser, City: city, State: state, Country: country}
	err := findVisitorData(&visitorData)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			err = createVisitorData(&visitorData)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		default:
			http.Error(w, err.Error(), 500)
			return
		}
	}

	json.NewEncoder(w).Encode(User{UserID: visitorData.UserID})
}

func getIpInfo(strIp string) *City {
	ip := net.ParseIP(strIp)
	if ip == nil {
		return nil
	}

	var ipInfo City
	err := geoIpDb.Lookup(ip, &ipInfo)
	if err != nil {
		log.Fatal(err)
	}
	return &ipInfo
}

func getBrowser(strBrowser string) string {
	return "CHROME"
}

func dbConfig() map[string]string {
	conf := make(map[string]string)
	conf[dbhost] = "localhost"
	conf[dbport] = "5432"
	conf[dbuser] = "postgres"
	conf[dbpass] = "a"
	conf[dbname] = "userid"
	return conf
}

func initDb() {
	config := dbConfig()
	var err error
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s "+
		"password=%s dbname=%s sslmode=disable",
		config[dbhost], config[dbport],
		config[dbuser], config[dbpass], config[dbname])

	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		panic(err)
	}
	err = db.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println("Successfully connected!")
}

func findVisitorData(visitorData *visitorData) error {
	sqlStatement := `
		SELECT 
			id,
			user_id
		FROM visitor_data
		WHERE finger_print=$1 and browser=$2 and city=$3 and state=$4 and country=$5
		LIMIT 1;`

	row := db.QueryRow(sqlStatement, visitorData.FingerPrint, visitorData.Browser, visitorData.City, visitorData.State, visitorData.Country)
	err := row.Scan(&visitorData.ID, &visitorData.UserID)
	if err != nil {
		return err
	}
	return nil
}

func createVisitorData(visitorData *visitorData) error {
	visitorData.UserID = xid.New().String()

	sqlStatement := `
		INSERT INTO visitor_data (finger_print, browser, city, state, country, user_id)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := db.Exec(sqlStatement, visitorData.FingerPrint, visitorData.Browser, visitorData.City, visitorData.State, visitorData.Country, visitorData.UserID)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	var err error
	geoIpDb, err = maxminddb.Open("geoip/GeoLite2-City.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer geoIpDb.Close()

	initDb()
	defer db.Close()

	router := mux.NewRouter()
	router.HandleFunc("/user-id", GetUserId).Queries("fingerPrint", "{fingerPrint}", "ip", "{ip}", "userAgent", "{userAgent}").Methods("GET")
	log.Fatal(http.ListenAndServe(":8000", router))
}

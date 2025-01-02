package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"go.bug.st/serial"
)

type EnvData struct {
	co2         int
	humidity    float64
	temperature float64
}

func parse(data string) (EnvData, error) {
	ret := EnvData{}

	data = strings.TrimSuffix(data, "\r\n")
	switch data {
	case "NG":
		return EnvData{}, errors.New("NG")
	case "OK STA":
		log.Println("OKOKOKOK")
		return EnvData{}, errors.New("start")
	case "OK STP":
		log.Println("OK stop")
		return EnvData{}, errors.New("stop")
	}

	sdata := strings.Split(data, ",")

	if !strings.HasPrefix(sdata[0], "CO2") {
		return EnvData{}, errors.New("unknown")
	}
	//log.Printf("%v", sdata)
	for _, sd := range sdata {
		var err error

		//log.Printf("%v", sd)
		//log.Printf("%v", sd[4:])
		switch sd[:3] {
		case "CO2":
			ret.co2, err = strconv.Atoi(sd[4:])
		case "HUM":
			ret.humidity, err = strconv.ParseFloat(sd[4:], 64)
		case "TMP":
			ret.temperature, err = strconv.ParseFloat(sd[4:], 64)
		}
		if err != nil {
			return EnvData{}, err
		}
	}

	return ret, nil
}

func insertData(db *sql.DB, device_id string, timestamp time.Time, data EnvData) error {
	// CO2
	query := `
		INSERT INTO co2 (time, device_id, concentration)
		VALUES ($1, $2, $3)`
	_, err := db.Exec(query, timestamp, device_id, data.co2)
	if err != nil {
		log.Println(err)
		return err
	}

	query = `
		INSERT INTO humidity (time, device_id, humidity)
		VALUES ($1, $2, $3)`
	_, err = db.Exec(query, timestamp, device_id, data.humidity)
	if err != nil {
		log.Println(err)
		return err
	}

	query = `
		INSERT INTO temperature (time, device_id, temperature)
		VALUES ($1, $2, $3)`
	_, err = db.Exec(query, timestamp, device_id, data.temperature)
	if err != nil {
		log.Println(err)
		return err
	}

	return nil
}

func run(db *sql.DB, device_id string) error {
	port, err := serial.Open("/dev/UD_CO2S", &serial.Mode{})
	if err != nil {
		return err
	}

	smode := &serial.Mode{
		BaudRate: 115200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	if err := port.SetMode(smode); err != nil {
		return err
	}

	err = db.Ping()
	if err != nil {
		log.Fatalf("failed to ping to database: %v", err)
	}

	// stop & clear buffer
	_, err = port.Write([]byte("STP\r\n"))
	time.Sleep(100 * time.Millisecond)
	err = port.ResetInputBuffer()

	// start
	_, err = port.Write([]byte("STA\r\n"))

	reader := bufio.NewReader(port)

	for {
		line, err := reader.ReadBytes('\n')
		timestamp := time.Now().UTC()
		if err != nil {
			log.Printf("failed to read data: %v", err)
			continue
		}
		data, err := parse(string(line))

		if err != nil {
			continue
		}
		log.Printf("%v: %v", timestamp, data)

		insertData(db, device_id, timestamp, data)
	}
}

func main() {
	var err error

	fmt.Println("Hello, Go!")

	// PostgreSQL host
	psql_host := os.Getenv("POSTGRES_HOST")
	if psql_host == "" {
		psql_host = "localhost"
	}

	// PostgreSQL port
	psql_port := 5432
	psql_port_s := os.Getenv("POSTGRES_PORT")
	if psql_port_s != "" {
		log.Println("PostgreSQL port does not specified. fallback to default: 5432")
		psql_port, err = strconv.Atoi(psql_port_s)
	}

	// PostgreSQL user
	psql_user := os.Getenv("POSTGRES_USER")
	if psql_user == "" {
		log.Println("PostgreSQL user does not specified. fallback to default: default")
		psql_user = "default"
	}

	// PostgreSQL password
	psql_password := os.Getenv("POSTGRES_PASSWORD")
	if psql_password == "" {
		log.Println("PostgreSQL password does not specified. fallback to default")
	}

	// PostgreSQL DB
	psql_db := os.Getenv("POSTGRES_DB")
	if psql_db == "" {
		log.Println("PostgreSQL DB does not specified. fallback to default")
	}

	// UD-CO2S serial ID
	serial_id := os.Getenv("UD_CO2S_SERIAL_ID")
	if serial_id == "" {
		log.Println("UD-CO2S serial ID does not specified. fallback to default: demo")
		serial_id = "demo"
	}

	fmt.Printf("postgres: %s\n", psql_host)

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}
	device_id := fmt.Sprintf("%s:%s", hostname, serial_id)
	log.Println(device_id)

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", psql_host, psql_port, psql_user, psql_password, psql_db)
	log.Printf("opening postgres: %s\n", psqlInfo)

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := run(db, device_id); err != nil {
		log.Fatal(err)
	}
}

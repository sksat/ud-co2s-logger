package main

import (
	"bufio"
	"database/sql"
	"errors"
	"flag"
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
		return EnvData{}, errors.ErrUnsupported
	case "OK STA":
		log.Println("OKOKOKOK")
		return EnvData{}, errors.New("start")
	}

	sdata := strings.Split(data, ",")
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
	fmt.Println("Hello, Go!")

	psql_host := flag.String("postgres", "default", "hoge")
	psql_port := flag.Int("postgres-port", 5432, "postgresql port")
	psql_user := flag.String("postgres-user", "default", "postgresql user")
	psql_password := flag.String("postgres-password", "default", "postgresql password")
	psql_db := flag.String("postgres-db", "", "postgresql DB")
	serial_id := flag.String("sensor-serial", "demo", "serial ID")
	flag.Parse()

	fmt.Printf("postgres: %s\n", *psql_host)

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}
	device_id := fmt.Sprintf("%s:%s", hostname, *serial_id)
	log.Println(device_id)


	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", *psql_host, *psql_port, *psql_user, *psql_password, *psql_db)
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

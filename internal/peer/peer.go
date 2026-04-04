package peer

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
)

func InitUUID() (uuid.UUID, error) {
	var id uuid.UUID
	data, err := os.ReadFile("uuid.txt")
	if err == nil {
		id, err = uuid.Parse(string(data))
		if err != nil {
			log.Fatalf("Failed to parse uuid: %v", err)
			return id, err
		}
		fmt.Println("There's already an uuid generated: ", id)
		return id, err
	} else if errors.Is(err, os.ErrNotExist) {
		id = uuid.New()
		err := os.WriteFile("uuid.txt", []byte(id.String()), 0644)
		if err != nil {
			log.Fatalf("Failed to write to file: %v\n", err)
			return id, err
		}
		fmt.Println("New uuid generated: ", id)
		return id, err
	} else {
		log.Fatalf("Unexpected error: %v\n", err)
		return id, err
	}
}

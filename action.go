package gameserver

import (
	"database/sql"
	"fmt"
	"log"
)

type Action struct {
	ActionNum int    `json:"action_num"`
	Action    string `json:"action"`
	Signature string `json:"signature"`
}

func saveAction(gameID int, actionNum int, action string, signature string) error {
	_, err := db.Exec("INSERT INTO actions(game_id, action_num, action, action_signature) VALUES(?, ?, ?, ?)",
		gameID, actionNum, action, signature)
	return err
}

func checkActionValidity(gameID int, actionNum int) error {
	numActions, err := GetNumberOfActions(gameID)
	if err != nil {
		return err
	}
	if actionNum != numActions+1 {
		return fmt.Errorf("invalid action number: got %d, expected %d", actionNum, numActions+1)
	}
	return nil
}

func GetNumberOfActions(gameID int) (int, error) {
	var numActions int
	err := db.QueryRow("SELECT COUNT(*) FROM actions WHERE game_id = ?", gameID).Scan(&numActions)
	if err != nil {
		return -1, err
	}
	return numActions, nil
}

func getAllActions(gameID int) ([]Action, error) {
	rows, err := db.Query("SELECT action_num, action, action_signature FROM actions WHERE game_id = ?", gameID)
	if err == sql.ErrNoRows {
		return []Action{}, nil
	} else if err != nil {
		return nil, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Printf("error closing rows: %v", err)
		}
	}(rows)

	var allActions []Action
	for rows.Next() {
		var action Action
		if err := rows.Scan(&action.ActionNum, &action.Action, &action.Signature); err != nil {
			return nil, err
		}
		allActions = append(allActions, action)
	}
	return allActions, nil
}

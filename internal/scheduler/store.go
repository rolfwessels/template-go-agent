package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ScheduleStore struct {
	dir string
}

func NewStore(dir string) *ScheduleStore {
	return &ScheduleStore{dir: dir}
}

func (s *ScheduleStore) Load(userID string) ([]*Schedule, error) {
	data, err := os.ReadFile(s.path(userID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading schedule file: %w", err)
	}
	var schedules []*Schedule
	if err := json.Unmarshal(data, &schedules); err != nil {
		return nil, fmt.Errorf("parsing schedule file: %w", err)
	}
	return schedules, nil
}

func (s *ScheduleStore) Save(userID string, schedules []*Schedule) error {
	dir := filepath.Dir(s.path(userID))
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating schedule dir: %w", err)
	}
	data, err := json.Marshal(schedules)
	if err != nil {
		return fmt.Errorf("marshaling schedules: %w", err)
	}
	if err := os.WriteFile(s.path(userID), data, 0600); err != nil {
		return fmt.Errorf("writing schedule file: %w", err)
	}
	return nil
}

func (s *ScheduleStore) LoadAll() (map[string][]*Schedule, error) {
	des, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading schedules dir: %w", err)
	}
	result := make(map[string][]*Schedule)
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		userID := de.Name()
		schedules, err := s.Load(userID)
		if err != nil {
			return nil, fmt.Errorf("loading schedules for %s: %w", userID, err)
		}
		if len(schedules) > 0 {
			result[userID] = schedules
		}
	}
	return result, nil
}

func (s *ScheduleStore) path(userID string) string {
	return filepath.Join(s.dir, userID, "schedule.json")
}

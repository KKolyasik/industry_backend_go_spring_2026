package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("task not found")

type Clock interface {
	Now() time.Time
}

type RequestTask struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Done      *bool     `json:"done"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Task struct {
	ID        string
	Title     string
	Done      bool
	UpdatedAt time.Time
}

type TaskRepo interface {
	Create(title string) (Task, error)
	Get(id string) (Task, bool)
	List() []Task
	SetDone(id string, done bool) (Task, error)
}

type InMemory struct {
	mu     sync.RWMutex
	data   map[string]Task
	clock  Clock
	currID int
}

func NewInMemoryTaskRepo(clock Clock) *InMemory {
	return &InMemory{
		data:  make(map[string]Task),
		clock: clock,
	}
}

func (m *InMemory) Create(title string) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currID++
	id := strconv.Itoa(m.currID)
	task := Task{
		ID:        id,
		Title:     title,
		Done:      false,
		UpdatedAt: m.clock.Now(),
	}
	m.data[id] = task

	return task, nil
}

func (m *InMemory) Get(id string) (Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.data[id]
	return task, ok
}

func (m *InMemory) List() []Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]Task, 0, len(m.data))
	for _, task := range m.data {
		list = append(list, task)
	}
	return list
}

func (m *InMemory) SetDone(id string, done bool) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.data[id]
	if !ok {
		return task, ErrNotFound
	}
	task.Done = done
	task.UpdatedAt = m.clock.Now()
	m.data[id] = task
	return task, nil
}

type HTTPHandler struct {
	repo TaskRepo
}

func (h *HTTPHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var t RequestTask
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&t)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(t.Title)
	if title == "" {
		http.Error(w, "Empty title", http.StatusBadRequest)
		return
	}
	task, err := h.repo.Create(title)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	enc := json.NewEncoder(w)
	enc.Encode(task)
}

func (h *HTTPHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	task, ok := h.repo.Get(id)
	if !ok {
		http.Error(w, "Task not dound", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	enc.Encode(task)
}

func (h *HTTPHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	tasks := h.repo.List()
	slices.SortFunc(tasks, func(a, b Task) int {
		if a.UpdatedAt.Equal(b.UpdatedAt) {
			aId, err := strconv.Atoi(a.ID)
			if err != nil {
				panic(err)
			}
			bId, err := strconv.Atoi(b.ID)
			if err != nil {
				panic(err)
			} 

			if aId < bId {
				return -1
			} else if bId < aId {
				return 1
			} else {
				return 0
			}
		} else if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		} else {
			return 1
		}
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	enc.Encode(tasks)
}

func (h *HTTPHandler) EditTasks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var t RequestTask
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&t)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	done := t.Done
	if done == nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	task, err := h.repo.SetDone(id, *done)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	enc.Encode(task)
}

func NewHTTPHandler(repo TaskRepo) http.Handler {
	hander := &HTTPHandler{repo: repo}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tasks", hander.CreateTask)
	mux.HandleFunc("GET /tasks", hander.GetTasks)
	mux.HandleFunc("GET /tasks/{id}", hander.GetTask)
	mux.HandleFunc("PATCH /tasks/{id}", hander.EditTasks)
	return mux
}

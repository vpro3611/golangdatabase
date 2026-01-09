package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"golangdb/errors_consts"
	"log"
	"net/http"
)

type SignUpAndLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type User struct {
	ID       int
	Email    string
	Password string
	IsAdmin  bool
}

type InsertRequest struct {
	Table  string         `json:"table"`
	Values map[string]any `json:"values"`
}

type SelectRequest struct {
	Table string        `json:"table"`
	Where *WhereRequest `json:"where,omitempty"`
}

type DeleteRequest struct {
	Table string `json:"table"`
	Where *WhereRequest
}

type WhereRequest struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

func (s *Server) SingUpHandler(w http.ResponseWriter, r *http.Request) {
	var req SignUpAndLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Println("Failed to decode: ", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	existing, err := s.Database.Select().Table("__users__").Where("email", "=", req.Email).All()
	if err != nil {
		log.Println("Failed to select user: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if len(existing) > 0 {
		http.Error(w, "User already exists", http.StatusBadRequest)
		return
	}

	encUserPass, err := EncryptPassword(req.Password)

	if err != nil {
		log.Println("Failed to encrypt password: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	userId, err := s.Database.Insert().Table("__users__").Values(map[string]any{
		"email":    req.Email,
		"password": encUserPass,
		"is_admin": false,
	}).ExecAndReturnID()

	if err != nil {
		log.Println("Failed to insert user: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	token, err := GenerateJWT(userId, false)
	if err != nil {
		log.Println("Failed to generate JWT: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	forEncoding := map[string]string{
		"token": token,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(forEncoding); err != nil {
		log.Println("Failed to encode: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (s *Server) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req SignUpAndLoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Println("Failed to decode: ", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	users, err := s.Database.Select().Table("__users__").Where("email", "=", req.Email).All()

	if err != nil {
		log.Println("Failed to select user: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}

	if err != nil {
		log.Println("Failed to decode: ", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(users) == 0 {
		http.Error(w, "User not found | Invalid credentials", http.StatusNotFound)
		return
	}

	user := users[0]

	hashedUserPass, ok := user["password"].(string)

	if !ok {
		log.Println("Failed to decode password: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := ComparePasswords(hashedUserPass, req.Password); err != nil {
		log.Println("Failed to compare passwords: ", err)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	num, ok := user["id"].(json.Number)
	if !ok {
		log.Println("Failed to decode id: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	id, err := num.Int64()
	if err != nil {
		log.Println("Failed to decode id: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	isAdmin, ok := user["is_admin"].(bool)
	if !ok {
		log.Println("Failed to decode is_admin: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	token, err := GenerateJWT(id, isAdmin)
	if err != nil {
		log.Println("Failed to generate JWT: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	forEncoding := map[string]string{
		"token": token,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(forEncoding); err != nil {
		log.Println("Failed to encode: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

}

// post

func (s *Server) InsertHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(*Claims)
	if !ok {
		log.Println("No user in context (insert handler)")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req InsertRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Println("Failed to decode: ", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	table := fmt.Sprintf("user:%d:%s", user.UserID, req.Table)

	err := s.Database.Insert().Table(table).Values(req.Values).Exec()

	if err != nil {
		log.Println("Failed to insert: ", err)
		if errors.Is(err, errors_consts.ErrEmptyName) || errors.Is(err, errors_consts.ErrEmptyValues) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// get

func (s *Server) SelectHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(*Claims)
	if !ok {
		log.Println("No user in context (select handler)")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req SelectRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Println("Failed to decode: ", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	var prefix string

	if user.IsAdmin {
		prefix = fmt.Sprintf("user:")
	} else {
		prefix = fmt.Sprintf("user:%d:", user.UserID)
	}

	table := prefix + req.Table

	query := s.Database.Select().Table(table)

	if req.Where != nil {
		query = query.Where(
			req.Where.Field,
			req.Where.Op,
			req.Where.Value,
		)
	}

	rows, err := query.All()

	if err != nil {
		log.Println("Failed to select: ", err)
		if errors.Is(err, errors_consts.ErrEmptyName) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(rows); err != nil {
		log.Println("Failed to encode: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// delete

func (s *Server) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userContextKey).(*Claims)

	if !ok {
		log.Println("No user in context (delete handler)")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req DeleteRequest

	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Println("Failed to decode: ", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	table := fmt.Sprintf("user:%d:%s", user.UserID, req.Table)

	query := s.Database.Delete().Table(table)

	if req.Where != nil {
		query = query.Where(
			req.Where.Field,
			req.Where.Op,
			req.Where.Value,
		)
	}

	if err := query.Exec(); err != nil {
		log.Println("Failed to delete: ", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

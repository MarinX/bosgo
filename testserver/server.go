package testserver

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	"github.com/bankrs/bosgo"
)

type Dev struct {
	ID string
}

type App struct {
	ID          string
	DeveloperID string
}

type User struct {
	ID            string
	Username      string
	Password      string
	ApplicationID string
	Accesses      []bosgo.Access
}

type Job struct {
	ID              string
	UserID          string
	ProviderID      string
	Stage           bosgo.JobStage
	Error           string
	SuppliedAnswers []bosgo.ChallengeAnswer
	AccessDetails   AccessDetails
	Succeeded       bool
}

type AccessDetails struct {
	Access       bosgo.Access
	ChallengeMap map[string]string
}

func (j *Job) isAnswered(id, val string) bool {
	for _, ans := range j.SuppliedAnswers {
		if ans.ID == id && ans.Value == val {
			return true
		}
	}
	return false
}

type Logger interface {
	Logf(format string, args ...interface{})
}

type Server struct {
	Svr *httptest.Server
	mux *http.ServeMux

	mu         sync.Mutex // guards following fields
	id         int64
	logger     Logger
	Devs       map[string]Dev           // map of developers indexed by ID
	Apps       map[string]App           // map of applications indexed by ID
	Users      map[string]User          // map of users indexed by ID
	UserTokens map[string]string        // map of tokens to user ID
	Jobs       map[string]Job           // map of jobs indexed by ID
	Accesses   map[string]AccessDetails // map of access details indexed by provider ID
}

func New() *Server {
	s := Server{
		Devs:       make(map[string]Dev),
		Apps:       make(map[string]App),
		Users:      make(map[string]User),
		UserTokens: make(map[string]string),
		Jobs:       make(map[string]Job),
		Accesses:   make(map[string]AccessDetails),
	}
	s.Svr = httptest.NewTLSServer(&s)

	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/v1/users", s.handleUsers)
	s.mux.HandleFunc("/v1/users/login", s.handleUsersLogin)
	s.mux.HandleFunc("/v1/users/logout", s.handleUsersLogout)
	s.mux.HandleFunc("/v1/users/reset_password", s.handleUsersResetPassword)

	s.mux.HandleFunc("/v1/accesses", s.handleAccesses)
	s.mux.HandleFunc("/v1/accounts", s.handleAccounts)
	s.mux.HandleFunc("/v1/jobs/", s.handleJobs)

	return &s
}

func (s *Server) URL() string {
	return s.Svr.URL
}

func (s *Server) Addr() string {
	u, _ := url.Parse(s.Svr.URL)

	return u.Host
}

func (s *Server) SetLogger(logger Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger = logger
}

func (s *Server) Logf(format string, args ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logger == nil {
		return
	}
	s.logger.Logf(format, args...)
}

func (s *Server) Client() *http.Client {
	cert, err := x509.ParseCertificate(s.Svr.TLS.Certificates[0].Certificate[0])
	if err != nil {
		panic(err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(cert)

	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
			},
		},
	}

	return &client
}

func (s *Server) Close() {
	s.Svr.Close()
	s.Svr = nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.mux.ServeHTTP(w, req)
}

type errorResp struct {
	Errors []apiError `json:"errors"`
}

// APIError represents an error that API may deliver to the customer
type apiError struct {
	Code    string                 `json:"code"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

func (s *Server) sendError(w http.ResponseWriter, status int, errcode string) {
	resp := errorResp{
		Errors: []apiError{
			{
				Code: errcode,
			},
		},
	}
	s.sendJSON(w, status, resp)
}

func (s *Server) sendJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	data, _ := json.Marshal(v)
	s.Logf("wrote: %s", data)
	fmt.Fprintf(w, "%s", data)
}

func (s *Server) sendNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) readJSON(w http.ResponseWriter, req *http.Request, v interface{}) bool {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		s.Logf("failed to read body: %v", err)
		s.sendError(w, http.StatusBadRequest, "general")
		return false
	}

	s.Logf("%s read: %s", req.URL.Path, string(body))
	if err := json.Unmarshal(body, v); err != nil {
		s.Logf("failed to unmarshal body: %v", err)
		s.sendError(w, http.StatusBadRequest, "general")
		return false
	}

	return true
}

func (s *Server) nextIDStr() string {
	id := s.nextID()
	return fmt.Sprintf("%08x", id)
}

func (s *Server) nextID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id++
	return s.id
}

func (s *Server) getApp(id string) (App, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Apps == nil {
		return App{}, false
	}
	app, exists := s.Apps[id]
	return app, exists
}

func (s *Server) setApp(app App) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Apps[app.ID] = app
}

func (s *Server) requireApp(w http.ResponseWriter, req *http.Request) (App, bool) {
	id := req.Header.Get("X-Application-Id")
	if id == "" {
		s.sendError(w, http.StatusUnauthorized, "authentication_app_id_invalid")
		return App{}, false
	}
	return s.getApp(id)
}

func (s *Server) getUser(id string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Users == nil {
		return User{}, false
	}
	user, exists := s.Users[id]
	return user, exists
}

func (s *Server) setUser(user User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user.Accesses == nil {
		user.Accesses = []bosgo.Access{}
	}
	s.Users[user.ID] = user
}

func (s *Server) requireUser(w http.ResponseWriter, req *http.Request) (User, string, bool) {
	app, exists := s.requireApp(w, req)
	if !exists {
		return User{}, "", false
	}

	token := req.Header.Get("X-Token")

	s.mu.Lock()
	id, exists := s.UserTokens[token]
	s.mu.Unlock()

	if !exists {
		s.sendError(w, http.StatusUnauthorized, "authentication_failed")
		return User{}, "", false
	}
	user, found := s.getUser(id)
	if !found || user.ApplicationID != app.ID {
		s.sendError(w, http.StatusUnauthorized, "authentication_failed")
		return User{}, "", false
	}

	return user, token, found
}

func (s *Server) setUserLoggedIn(id string) string {
	token := s.nextIDStr()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UserTokens[token] = id
	return token
}

func (s *Server) setUserLoggedOut(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.UserTokens, token)
}

func (s *Server) newJob(userID string, providerID string, answers []bosgo.ChallengeAnswer) *bosgo.Job {
	job := Job{
		ID:         s.nextIDStr(),
		UserID:     userID,
		ProviderID: providerID,
		Stage:      bosgo.JobStageAuthenticating,
	}

	s.mu.Lock()
	ad, exists := s.Accesses[providerID]
	s.mu.Unlock()
	if !exists {
		job.Stage = bosgo.JobStageFinished
		job.Error = "unknown_provider"
	} else {
		job.AccessDetails = ad
		s.progressJob(&job, answers)
	}

	s.mu.Lock()
	s.Jobs[job.ID] = job
	s.mu.Unlock()

	return &bosgo.Job{
		URI: "/jobs/" + job.ID,
	}

}

func (s *Server) setJob(job Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Jobs[job.ID] = job
}

func (s *Server) getJob(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, exists := s.Jobs[id]
	return job, exists
}

func (s *Server) requireJob(w http.ResponseWriter, req *http.Request) (Job, bool) {
	user, _, found := s.requireUser(w, req)
	if !found {
		return Job{}, false
	}
	if !strings.HasPrefix(req.URL.Path, "/v1/jobs/") {
		s.sendError(w, http.StatusBadRequest, "general")
		return Job{}, false
	}
	jobID := req.URL.Path[9:]

	job, exists := s.getJob(jobID)
	if !exists {
		s.sendError(w, http.StatusNotFound, "resource_not_found")
		return Job{}, false
	}
	if job.UserID != user.ID {
		s.sendError(w, http.StatusUnauthorized, "authentication_failed")
		return Job{}, false
	}

	return job, true
}

func (s *Server) progressJob(j *Job, answers []bosgo.ChallengeAnswer) {
	j.SuppliedAnswers = append(j.SuppliedAnswers, answers...)

	for id, val := range j.AccessDetails.ChallengeMap {
		if !j.isAnswered(id, val) {
			return
		}
	}

	j.Stage = bosgo.JobStageFinished
	j.Succeeded = true

	user, found := s.getUser(j.UserID)
	if !found {
		return
	}
	user.Accesses = append(user.Accesses, j.AccessDetails.Access)
	s.setUser(user)
}

// AddAccess adds configuration for an access so it can be added to a user via the server API
func (s *Server) AddAccess(access *bosgo.Access, challengeMap map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ad := AccessDetails{
		Access:       *access,
		ChallengeMap: challengeMap,
	}
	s.Accesses[access.ProviderID] = ad
}

func (s *Server) handleUsers(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		s.handlerUserCreate(w, req)
		return
	case http.MethodDelete:
		s.handlerUserDelete(w, req)
		return
	}

	http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
	return
}

func (s *Server) handlerUserCreate(w http.ResponseWriter, req *http.Request) {
	app, proceed := s.requireApp(w, req)
	if !proceed {
		return
	}

	defer req.Body.Close()
	var creds bosgo.UserCredentials
	if !s.readJSON(w, req, &creds) {
		return
	}

	s.mu.Lock()
	for _, u := range s.Users {
		if u.Username == creds.Username {
			s.mu.Unlock()
			s.sendError(w, http.StatusInternalServerError, "server_side")
			return
		}
	}
	s.mu.Unlock()

	user := User{
		ID:            s.nextIDStr(),
		Password:      creds.Password,
		ApplicationID: app.ID,
	}

	s.setUser(user)
	token := s.setUserLoggedIn(user.ID)

	ut := bosgo.UserToken{
		ID:    user.ID,
		Token: token,
	}
	s.sendJSON(w, http.StatusCreated, &ut)
}

func (s *Server) handlerUserDelete(w http.ResponseWriter, req *http.Request) {
	s.sendError(w, http.StatusInternalServerError, "not_implemented_by_test_server")
}

func (s *Server) handleUsersLogin(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	app, proceed := s.requireApp(w, req)
	if !proceed {
		return
	}

	var creds bosgo.UserCredentials
	if !s.readJSON(w, req, &creds) {
		return
	}

	s.mu.Lock()
	var user User
	for _, u := range s.Users {
		if u.Username != creds.Username {
			continue
		}
		if u.Password != creds.Password {
			break
		}
		user = u
		break
	}
	s.mu.Unlock()

	if user.ApplicationID != app.ID {
		s.sendError(w, http.StatusUnauthorized, "authentication_failed")
		return
	}

	token := s.setUserLoggedIn(user.ID)
	ut := bosgo.UserToken{
		ID:    user.ID,
		Token: token,
	}
	s.sendJSON(w, http.StatusOK, &ut)
}

func (s *Server) handleUsersLogout(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	_, token, found := s.requireUser(w, req)
	if !found {
		return
	}
	s.setUserLoggedOut(token)
	s.sendNoContent(w)
}

func (s *Server) handleUsersResetPassword(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	s.sendError(w, http.StatusInternalServerError, "not_implemented_by_test_server")
}

func (s *Server) handleAccesses(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		s.handlerAccessCreate(w, req)
		return
	case http.MethodGet:
		s.handlerAccessesList(w, req)
		return
	}

	http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
	return
}

func (s *Server) handlerAccessCreate(w http.ResponseWriter, req *http.Request) {
	user, _, found := s.requireUser(w, req)
	if !found {
		return
	}

	var data struct {
		ProviderID string                  `json:"provider_id"`
		Answers    []bosgo.ChallengeAnswer `json:"challenge_answers"`
	}
	if !s.readJSON(w, req, &data) {
		return
	}

	job := s.newJob(user.ID, data.ProviderID, data.Answers)

	s.sendJSON(w, http.StatusAccepted, &job)
}

func (s *Server) handlerAccessesList(w http.ResponseWriter, req *http.Request) {
	user, _, found := s.requireUser(w, req)
	if !found {
		return
	}

	s.sendJSON(w, http.StatusOK, user.Accesses)
}

func (s *Server) handleJobs(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		s.handlerJobStatus(w, req)
		return
	case http.MethodPut:
		s.handlerJobAnswer(w, req)
		return
	case http.MethodDelete:
		s.handlerJobDelete(w, req)
		return
	}

	http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
	return
}

func (s *Server) handlerJobStatus(w http.ResponseWriter, req *http.Request) {
	job, found := s.requireJob(w, req)
	if !found {
		return
	}
	s.sendJSON(w, http.StatusOK, s.jobStatus(&job))
}

func (s *Server) handlerJobAnswer(w http.ResponseWriter, req *http.Request) {
	job, found := s.requireJob(w, req)
	if !found {
		return
	}

	var answers struct {
		Answers []bosgo.ChallengeAnswer `json:"challenge_answers"`
	}
	if !s.readJSON(w, req, &answers) {
		return
	}

	s.progressJob(&job, answers.Answers)
	s.setJob(job)

	s.sendJSON(w, http.StatusOK, s.jobStatus(&job))
}

func (s *Server) handlerJobDelete(w http.ResponseWriter, req *http.Request) {
	job, found := s.requireJob(w, req)
	if !found {
		return
	}
	_ = job
	s.sendError(w, http.StatusInternalServerError, "not_implemented_by_test_server")
}

func (s *Server) jobStatus(job *Job) *bosgo.JobStatus {
	status := bosgo.JobStatus{
		Finished: job.Stage == bosgo.JobStageFinished,
		Stage:    job.Stage,
		URI:      "/jobs/" + job.ID,
		// Challenge *Challenge `json:"challenge,omitempty"`
	}

	if job.Error != "" {
		status.Errors = []bosgo.APIError{{Code: job.Error}}
	}

	if job.Succeeded {
		status.Access = &bosgo.JobAccess{
			ID:         job.AccessDetails.Access.ID,
			ProviderID: job.AccessDetails.Access.ProviderID,
			Name:       job.AccessDetails.Access.Name,
		}
		for _, ac := range job.AccessDetails.Access.Accounts {
			status.Access.Accounts = append(status.Access.Accounts, bosgo.JobAccount{
				ID:        ac.ID,
				Name:      ac.Name,
				Number:    ac.Number,
				IBAN:      ac.IBAN,
				Supported: ac.Supported,
			})
		}

	}

	return &status
}

func (s *Server) handleAccounts(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	user, _, found := s.requireUser(w, req)
	if !found {
		return
	}

	var page bosgo.AccountPage
	for _, acc := range user.Accesses {
		page.Accounts = append(page.Accounts, acc.Accounts...)
	}
	s.sendJSON(w, http.StatusOK, &page)
}

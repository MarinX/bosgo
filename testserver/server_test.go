package testserver

import (
	"testing"

	"github.com/bankrs/bosgo"
)

func TestUserLogin(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	_, err := appClient.Users.Login(DefaultUsername, DefaultPassword).Send()
	if err != nil {
		t.Fatalf("failed to login as user: %v", err)
	}
}

func TestUserLoginFail(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	_, err := appClient.Users.Login(DefaultUsername, "foo").Send()
	if err == nil {
		t.Fatalf("got no error, wanted one")
	}
}

func TestUserLoginWrongApp(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), "fooapp")
	_, err := appClient.Users.Login(DefaultUsername, DefaultPassword).Send()
	if err == nil {
		t.Fatalf("got no error, wanted one")
	}
}

func TestAccessesList(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	userClient, err := appClient.Users.Login(DefaultUsername, DefaultPassword).Send()
	if err != nil {
		t.Fatalf("failed to login as user: %v", err)
	}

	ac, err := userClient.Accesses.List().Send()
	if err != nil {
		t.Fatalf("failed to retrieve accesses: %v", err)
	}
	if len(ac.Accesses) != 0 {
		t.Errorf("got %d accesses, wanted 0", len(ac.Accesses))
	}

}

func TestUserCreate(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	userClient, err := appClient.Users.Create("scooby", "sandwich").Send()
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Test user is authorised to see their accesses (even though there are none for the new user)
	ac, err := userClient.Accesses.List().Send()
	if err != nil {
		t.Fatalf("failed to retrieve accesses: %v", err)
	}
	if len(ac.Accesses) != 0 {
		t.Errorf("got %d accesses, wanted 0", len(ac.Accesses))
	}

}

func TestAccessCreateNoLogin(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	userClient, err := appClient.Users.Login(DefaultUsername, DefaultPassword).Send()
	if err != nil {
		t.Fatalf("failed to login as user: %v", err)
	}

	job, err := userClient.Accesses.Add(DefaultProviderID).Send()
	if err != nil {
		t.Fatalf("failed to add access: %v", err)
	}
	t.Logf("job URI: %s", job.URI)

	status, err := userClient.Jobs.Get(job.URI).Send()
	if err != nil {
		t.Fatalf("failed to get job status: %v", err)
	}

	if status.Finished != false {
		t.Errorf("got finished %v, wanted false", status.Finished)
	}

	if status.Stage != bosgo.JobStageAuthenticating {
		t.Errorf("got stage %v, wanted %v", status.Stage, bosgo.JobStageAuthenticating)
	}
}

func TestAccessCreateWithLogin(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	userClient, err := appClient.Users.Login(DefaultUsername, DefaultPassword).Send()

	if err != nil {
		t.Fatalf("failed to login as user: %v", err)
	}

	req := userClient.Accesses.Add(DefaultProviderID)

	req.ChallengeAnswer(bosgo.ChallengeAnswer{
		ID:    "login",
		Value: DefaultAccessLogin,
	})
	req.ChallengeAnswer(bosgo.ChallengeAnswer{
		ID:    "pin",
		Value: DefaultAccessPIN,
	})

	job, err := req.Send()
	if err != nil {
		t.Fatalf("failed to add access: %v", err)
	}
	t.Logf("job URI: %s", job.URI)

	status, err := userClient.Jobs.Get(job.URI).Send()
	if err != nil {
		t.Fatalf("failed to get job status: %v", err)
	}

	if status.Finished != true {
		t.Errorf("got finished %v, wanted true", status.Finished)
	}
	if status.Stage != bosgo.JobStageFinished {
		t.Errorf("got stage %v, wanted %v", status.Stage, bosgo.JobStageFinished)
	}
	if status.Access == nil {
		t.Fatalf("got nil access, wanted non-nil")
	}

	if status.Access.ProviderID != DefaultProviderID {
		t.Errorf("got provider id %s, wanted %s", status.Access.ProviderID, DefaultProviderID)
	}
}

func TestAccessCreateUnknownProvider(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	userClient, err := appClient.Users.Login(DefaultUsername, DefaultPassword).Send()
	if err != nil {
		t.Fatalf("failed to login as user: %v", err)
	}

	providerID := "bogus_provider"
	job, err := userClient.Accesses.Add(providerID).Send()
	if err != nil {
		t.Fatalf("failed to add access: %v", err)
	}
	t.Logf("job URI: %s", job.URI)

	status, err := userClient.Jobs.Get(job.URI).Send()
	if err != nil {
		t.Fatalf("failed to get job status: %v", err)
	}

	if status.Finished != true {
		t.Errorf("got finished %v, wanted true", status.Finished)
	}

	if status.Stage != bosgo.JobStageFinished {
		t.Errorf("got stage %v, wanted %v", status.Stage, bosgo.JobStageFinished)
	}

	if len(status.Errors) == 0 {
		t.Errorf("got no errors, wanted at least one")
	}
}

func TestAccessCreateMultiStep(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	userClient, err := appClient.Users.Login(DefaultUsername, DefaultPassword).Send()

	if err != nil {
		t.Fatalf("failed to login as user: %v", err)
	}

	job, err := userClient.Accesses.Add(DefaultProviderID).Send()
	if err != nil {
		t.Fatalf("failed to add access: %v", err)
	}
	t.Logf("job URI: %s", job.URI)

	status, err := userClient.Jobs.Get(job.URI).Send()
	if err != nil {
		t.Fatalf("failed to get job status: %v", err)
	}
	if status.Stage != bosgo.JobStageAuthenticating {
		t.Errorf("got stage %v, wanted %v", status.Stage, bosgo.JobStageAuthenticating)
	}

	req := userClient.Jobs.Answer(job.URI)
	req.ChallengeAnswer(bosgo.ChallengeAnswer{
		ID:    "login",
		Value: DefaultAccessLogin,
	})
	if err := req.Send(); err != nil {
		t.Fatalf("failed to answer first challenge: %v", err)
	}
	status, err = userClient.Jobs.Get(job.URI).Send()
	if err != nil {
		t.Fatalf("failed to get job status: %v", err)
	}
	if status.Stage != bosgo.JobStageAuthenticating {
		t.Errorf("got stage %v, wanted %v", status.Stage, bosgo.JobStageAuthenticating)
	}

	req = userClient.Jobs.Answer(job.URI)
	req.ChallengeAnswer(bosgo.ChallengeAnswer{
		ID:    "pin",
		Value: DefaultAccessPIN,
	})

	if err := req.Send(); err != nil {
		t.Fatalf("failed to answer second challenge: %v", err)
	}
	status, err = userClient.Jobs.Get(job.URI).Send()
	if err != nil {
		t.Fatalf("failed to get job status: %v", err)
	}
	if status.Stage != bosgo.JobStageFinished {
		t.Errorf("got stage %v, wanted %v", status.Stage, bosgo.JobStageFinished)
	}
	if status.Finished != true {
		t.Errorf("got finished %v, wanted true", status.Finished)
	}

}

func TestAccessCreateAddsAccessToList(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	userClient, err := appClient.Users.Login(DefaultUsername, DefaultPassword).Send()

	if err != nil {
		t.Fatalf("failed to login as user: %v", err)
	}

	req := userClient.Accesses.Add(DefaultProviderID)

	req.ChallengeAnswer(bosgo.ChallengeAnswer{
		ID:    "login",
		Value: DefaultAccessLogin,
	})
	req.ChallengeAnswer(bosgo.ChallengeAnswer{
		ID:    "pin",
		Value: DefaultAccessPIN,
	})

	job, err := req.Send()
	if err != nil {
		t.Fatalf("failed to add access: %v", err)
	}
	t.Logf("job URI: %s", job.URI)

	ac, err := userClient.Accesses.List().Send()
	if err != nil {
		t.Fatalf("failed to retrieve accesses: %v", err)
	}
	if len(ac.Accesses) != 1 {
		t.Errorf("got %d accesses, wanted 1", len(ac.Accesses))
	}
}

func TestAccessCreateEnabledGetAccess(t *testing.T) {
	s := NewWithDefaults()
	if testing.Verbose() {
		s.SetLogger(t)
	}
	defer s.Close()

	appClient := bosgo.NewAppClient(s.Client(), s.Addr(), DefaultApplicationID)
	userClient, err := appClient.Users.Login(DefaultUsername, DefaultPassword).Send()

	if err != nil {
		t.Fatalf("failed to login as user: %v", err)
	}

	req := userClient.Accesses.Add(DefaultProviderID)

	req.ChallengeAnswer(bosgo.ChallengeAnswer{
		ID:    "login",
		Value: DefaultAccessLogin,
	})
	req.ChallengeAnswer(bosgo.ChallengeAnswer{
		ID:    "pin",
		Value: DefaultAccessPIN,
	})

	job, err := req.Send()
	if err != nil {
		t.Fatalf("failed to add access: %v", err)
	}
	t.Logf("job URI: %s", job.URI)

	status, err := userClient.Jobs.Get(job.URI).Send()
	if err != nil {
		t.Fatalf("failed to get job status: %v", err)
	}
	if status.Access == nil {
		t.Fatalf("got nil access, wanted non-nil")
	}

	acc, err := userClient.Accesses.Get(status.Access.ID).Send()
	if err != nil {
		t.Fatalf("failed to retrieve accesses: %v", err)
	}
	if acc.Name != "default access" {
		t.Errorf("got access %s, wanted %s", acc.Name, "default access")
	}
}

package saml

import (
	"bytes"
	"github.com/RobotsAndPencils/go-saml"
	"github.com/dropbox/godropbox/errors"
	"github.com/pritunl/pritunl-auth/constants"
	"github.com/pritunl/pritunl-auth/database"
	"github.com/pritunl/pritunl-auth/utils"
	"html/template"
	"path/filepath"
)

var (
	SamlCallbackUrl string
)

type UserData struct {
	Username string
	Email    string
	Org      string
	HasDuo   bool
}

type Token struct {
	Id             string `bson:"_id"`
	RemoteCallback string `bson:"remote_callback"`
	RemoteState    string `bson:"remote_state"`
	RemoteSecret   string `bson:"remote_secret"`
	SsoUrl         string `bson:"sso_url"`
	IssuerUrl      string `bson:"issuer_url"`
	Cert           string `bson:"cert"`
}

type Saml struct {
	SsoUrl    string
	IssuerUrl string
	Cert      string
	provider  saml.ServiceProviderSettings
}

func (s *Saml) Init() (err error) {
	certPath := GetCertPath()
	err = utils.Write(certPath, s.Cert)
	if err != nil {
		return
	}

	s.provider = &saml.ServiceProviderSettings{
		PublicCertPath: filepath.Join(
			constants.SamlCertDir, constants.SamlCert),
		PrivateKeyPath: filepath.Join(
			constants.SamlCertDir, constants.SamlKey),
		IDPSSOURL:                   s.SsoUrl,
		IDPSSODescriptorURL:         s.IssuerUrl,
		IDPPublicCertPath:           certPath,
		SPSignRequest:               true,
		AssertionConsumerServiceURL: SamlCallbackUrl,
	}

	err = s.provider.Init()
	if err != nil {
		err = &constants.ReadError{
			errors.Wrap(err, "saml: Failed to init provider"),
		}
		return
	}

	return
}

func (s *Saml) Request(db *database.Database, remoteState, remoteSecret,
	remoteCallback string) (resp *bytes.Buffer, err error) {

	coll := db.Tokens()
	state := utils.RandStr(32)

	req := s.provider.GetAuthnRequest()
	encodedReq, err := req.EncodedSignedString(s.provider.PrivateKeyPath)
	if err != nil {
		err = &SamlError{
			errors.Wrap(err, "saml: Encode error"),
		}
		return
	}

	data := struct {
		SsoUrl      string
		SAMLRequest string
		RelayState  string
	}{
		SsoUrl:      s.provider.IDPSSOURL,
		SAMLRequest: encodedReq,
		RelayState:  state,
	}

	respTemplate := template.New("saml")
	respTemplate, err = respTemplate.Parse(bindTemplate)
	if err != nil {
		err = &SamlError{
			errors.Wrap(err, "saml: Template parse error"),
		}
		return
	}

	tokn := &Token{
		Id:             state,
		RemoteCallback: remoteCallback,
		RemoteState:    remoteState,
		RemoteSecret:   remoteSecret,
		SsoUrl:         s.SsoUrl,
		IssuerUrl:      s.IssuerUrl,
		Cert:           s.Cert,
		Type:           "saml",
	}
	err = coll.Insert(tokn)
	if err != nil {
		err = database.ParseError(err)
		return
	}

	resp = &bytes.Buffer{}
	err = respTemplate.Execute(resp, data)
	if err != nil {
		err = &SamlError{
			errors.Wrap(err, "saml: Template execute error"),
		}
		return
	}

	return
}

func (s *Saml) Authorize(db *database.Database, state, response string) (
	data *UserData, tokn *Token, err error) {

	resp, err := saml.ParseEncodedResponse(response)
	if err != nil {
		err = &SamlError{
			errors.Wrap(err, "saml: Failed to parse response"),
		}
		return
	}

	err = resp.Validate(&s.provider)
	if err != nil {
		err = &SamlError{
			errors.Wrap(err, "saml: Failed to validate response"),
		}
		return
	}

	data = &UserData{
		Username: resp.GetAttribute("username"),
		Email:    resp.GetAttribute("email"),
		Org:      resp.GetAttribute("org"),
	}

	if data.Username == "" {
		data.Username = resp.Assertion.Subject.NameID.Value
	}

	if resp.GetAttribute("has_duo") == "true" {
		data.HasDuo = true
	}

	return
}
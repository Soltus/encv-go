package alistencrypt

type AlistEncryptPluginConfig struct {
	Suffix          string `json:"suffix"`
	DefaultPassword string `json:"default_password"`
	EncType         string `json:"enc_type"`
}

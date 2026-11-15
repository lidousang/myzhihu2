package conf

// JWT contains jwt signing and expiration settings.
type JWT struct {
	// jwt: fixed signing algorithm accepted by the middleware.
	Algorithm string `yaml:"algorithm"`
	// jwt: issuer written to and checked from token claims.
	Issuer string `yaml:"issuer"`
	// jwt: server-side signing secret; never expose it in an API response.
	Secret string `yaml:"secret"`
	// jwt: access token lifetime measured in hours.
	ExpireHours int `yaml:"expire_hours"`
}

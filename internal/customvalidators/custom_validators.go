package customvalidators

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

var (
	// '*' is permitted so ACL groups can name an aggregate principal
	// (e.g. "*@cluster.ror.system").
	nameRegex = `^[@()\/:?\r\n.,*a-zA-Z æøåÆØÅ0-9_-]+$`
	regTester *regexp.Regexp
)

func Setup(validate *validator.Validate) {
	regTester, _ = regexp.Compile(nameRegex)
	_ = validate.RegisterValidation("rortext", ValidateText)
}

func ValidateText(fl validator.FieldLevel) bool {
	text := fl.Field().String()
	result := regTester.MatchString(text)
	return result
}

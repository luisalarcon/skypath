package api

import (
	"net/url"
	"strings"

	"github.com/luisalarcon/skypath/backend/internal/flight"
)

// Field names and error codes of the validation errors in docs/api.md.
const (
	fieldOrigin      = "origin"
	fieldDestination = "destination"
	fieldDate        = "date"

	codeRequired      = "required"
	codeInvalidFormat = "invalid_format"
	codeUnknownAir    = "unknown_airport"
	codeSameAsOrigin  = "same_as_origin"
)

// searchRequest is a validated search: the query for the searcher plus the
// airports it resolved to, which the response echoes back.
type searchRequest struct {
	query       flight.Query
	origin      flight.Airport
	destination flight.Airport
}

// validateSearch checks the query parameters of a search request. It reports
// every problem at once, in the order origin, destination, date, so a client
// can show them together. If a parameter is repeated, the first value is
// used; unknown parameters are ignored.
func validateSearch(params url.Values, airports AirportDirectory) (searchRequest, []fieldError) {
	var errs []fieldError

	origin, originErr := validateAirport(params.Get(fieldOrigin), fieldOrigin, "Origin", airports)
	if originErr != nil {
		errs = append(errs, *originErr)
	}

	destination, destErr := validateAirport(params.Get(fieldDestination), fieldDestination, "Destination", airports)
	if destErr != nil {
		errs = append(errs, *destErr)
	}
	// The pair is only compared once each side is a known airport, so a
	// request is never told both that a code is bad and that it is a repeat.
	if originErr == nil && destErr == nil && origin.Code == destination.Code {
		errs = append(errs, fieldError{
			Field:   fieldDestination,
			Code:    codeSameAsOrigin,
			Message: "Destination must be different from origin.",
		})
	}

	date, dateErr := validateDate(params.Get(fieldDate))
	if dateErr != nil {
		errs = append(errs, *dateErr)
	}

	if len(errs) > 0 {
		return searchRequest{}, errs
	}
	return searchRequest{
		query:       flight.Query{Origin: origin.Code, Destination: destination.Code, Date: date},
		origin:      origin,
		destination: destination,
	}, nil
}

// validateAirport applies the first failing check for an airport field:
// present, shaped like an IATA code, then known to the schedule. Codes are
// trimmed and upper-cased first, so " jfk " is accepted.
func validateAirport(raw, field, label string, airports AirportDirectory) (flight.Airport, *fieldError) {
	if strings.TrimSpace(raw) == "" {
		return flight.Airport{}, &fieldError{Field: field, Code: codeRequired, Message: label + " is required."}
	}
	code := flight.NormalizeCode(raw)
	if !flight.ValidCode(code) {
		return flight.Airport{}, &fieldError{Field: field, Code: codeInvalidFormat, Message: label + " must be a three-letter airport code."}
	}
	airport, ok := airports.Airport(code)
	if !ok {
		// code is three ASCII letters at this point, so it is safe to echo.
		return flight.Airport{}, &fieldError{Field: field, Code: codeUnknownAir, Message: "Unknown airport code " + code + "."}
	}
	return airport, nil
}

// validateDate requires a real calendar date in YYYY-MM-DD form. Unlike the
// airport codes it is not trimmed: docs/api.md specifies trimming only for
// codes, and a date with stray spaces is better rejected than guessed at.
func validateDate(raw string) (flight.Date, *fieldError) {
	if strings.TrimSpace(raw) == "" {
		return flight.Date{}, &fieldError{Field: fieldDate, Code: codeRequired, Message: "Date is required."}
	}
	date, err := flight.ParseDate(raw)
	if err != nil {
		return flight.Date{}, &fieldError{Field: fieldDate, Code: codeInvalidFormat, Message: "Date must be a valid date in YYYY-MM-DD format."}
	}
	return date, nil
}

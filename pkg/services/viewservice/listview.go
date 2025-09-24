package viewservice

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
	"github.com/gin-gonic/gin"
)

var ErrViewNotRegistered = errors.New("view not registered")

type ViewGenerator interface {
	GenerateView(ctx context.Context, opts ...ViewGeneratorsOption) (apiview.View, error)
	GetMetadata() apiview.ViewMetadata
}

// @Router			/v2/views/{viewid} [get]
// @Param			viewid		path	string							true	"The ID of the view to retrieve"
// @Param			limit	query	int							false	"Number of items to return, if set to -1, only metadata is returned"
// @Param			offset		query	int							false	"Number of items to skip before starting to collect the result set"
// @Param			sort		query	string							false	"Comma separated list of fields to sort by (e.g. name,-date)"
// @Param			filter		query	string							false	"Filter expression (e.g. name==example*,date>2020-01-01)"
// @Param			fields		query	string							false	"Comma separated list of extra fields to include in the response (e.g. workorder,branch,testfield1)"

type ViewGenerators map[string]ViewGenerator

var Generators = ViewGenerators{}

type viewGeneratorOptions struct {
	limit  int
	offset int
	sort   map[int]viewGeneratorOptionSort
	filter map[int]string
	fields []string
}

type viewGeneratorOptionSort struct {
	name string
	desc bool
}

type ViewGeneratorsOption interface {
	apply(*viewGeneratorOptions)
}

type optionFunc func(*viewGeneratorOptions)

func (of optionFunc) apply(cfg *viewGeneratorOptions) { of(cfg) }

func OptionLimit(limit int) ViewGeneratorsOption {
	return optionFunc(func(cfg *viewGeneratorOptions) {
		cfg.limit = limit
	})
}

func OptionOffset(offset int) ViewGeneratorsOption {
	return optionFunc(func(cfg *viewGeneratorOptions) {
		cfg.offset = offset
	})
}
func OptionSort(sort string) ViewGeneratorsOption {
	return optionFunc(func(cfg *viewGeneratorOptions) {
		cfg.sort = make(map[int]viewGeneratorOptionSort) // Initialize the map
		for i, item := range strings.Split(sort, ",") {
			if strings.HasPrefix(item, "-") {
				cfg.sort[i] = viewGeneratorOptionSort{
					name: strings.TrimPrefix(item, "-"),
					desc: true,
				}
			} else {
				cfg.sort[i] = viewGeneratorOptionSort{
					name: strings.TrimPrefix(item, "+"),
					desc: false,
				}
			}
		}
	})
}
func OptionFilter(filter string) ViewGeneratorsOption {
	return optionFunc(func(cfg *viewGeneratorOptions) {
		cfg.filter = make(map[int]string) // Initialize the map
		for i, item := range strings.Split(filter, ",") {
			cfg.filter[i] = item
		}
	})
}
func OptionFields(fields string) ViewGeneratorsOption {
	return optionFunc(func(cfg *viewGeneratorOptions) {
		cfg.fields = strings.Split(fields, ",")
	})
}

func (lv *ViewGenerators) RegisterViewGenerator(listType string, generator ViewGenerator) {
	(*lv)[listType] = generator
}
func (lv *ViewGenerators) UnregisterViewGenerator(listType string) {
	delete(*lv, listType)
}

func (lv *ViewGenerators) IsRegistered(listType string) bool {
	_, exists := (*lv)[listType]
	return exists
}

func (lv *ViewGenerators) GetGenerator(listType string) (ViewGenerator, error) {
	if !lv.IsRegistered(listType) {
		return nil, ErrViewNotRegistered
	}
	return (*lv)[listType], nil
}

// ParseOptionsFromGinContext parses the provided options from a gin context and returns an array of ViewGeneratorsOption
func ParseOptionsFromGinContext(c *gin.Context) []ViewGeneratorsOption {
	var options []ViewGeneratorsOption

	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			options = append(options, OptionLimit(limit))
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			options = append(options, OptionOffset(offset))
		}
	}

	if sort := c.Query("sort"); sort != "" {
		options = append(options, OptionSort(sort))
	}

	if filter := c.Query("filter"); filter != "" {
		options = append(options, OptionFilter(filter))
	}

	if fields := c.Query("fields"); fields != "" {
		options = append(options, OptionFields(fields))
	}

	return options
}

package db_repo

import (
	"context"
	"fmt"
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/mon"
	"github.com/applike/gosoline/pkg/tracing"
	"github.com/jinzhu/gorm"
	"github.com/jonboulle/clockwork"
	"reflect"
	"strconv"
	"strings"
)

const (
	Create = "create"
	Read   = "read"
	Update = "update"
	Delete = "delete"
	Query  = "query"
)

var operations = []string{Create, Read, Update, Delete, Query}
var RecordNotFound = gorm.ErrRecordNotFound

type Settings struct {
	cfg.AppId
	Metadata Metadata
}

//go:generate mockery -name Repository
type Repository interface {
	Create(ctx context.Context, value ModelBased) error
	Read(ctx context.Context, id *uint, out ModelBased) error
	Update(ctx context.Context, value ModelBased) error
	Delete(ctx context.Context, value ModelBased) error
	Query(ctx context.Context, qb *QueryBuilder, result interface{}) error
	Count(ctx context.Context, qb *QueryBuilder, model ModelBased) (int, error)

	GetModelId() string
	GetModelName() string
	GetMetadata() Metadata
}

type repository struct {
	logger   mon.Logger
	tracer   tracing.Tracer
	orm      *gorm.DB
	clock    clockwork.Clock
	settings Settings
}

func New(config cfg.Config, logger mon.Logger, s Settings) *repository {
	tracer := tracing.NewAwsTracer(config)
	orm := NewOrm(config, logger)
	clock := clockwork.NewRealClock()

	s.PadFromConfig(config)

	return NewWithInterfaces(logger, tracer, orm, clock, s)
}

func NewWithInterfaces(logger mon.Logger, tracer tracing.Tracer, orm *gorm.DB, clock clockwork.Clock, settings Settings) *repository {
	return &repository{
		logger:   logger,
		tracer:   tracer,
		orm:      orm,
		clock:    clock,
		settings: settings,
	}
}

func (r *repository) Create(ctx context.Context, value ModelBased) error {
	modelId := r.GetModelId()
	logger := r.logger.WithContext(ctx)

	ctx, span := r.startSubSpan(ctx, "Create")
	defer span.Finish()

	now := r.clock.Now()
	value.SetUpdatedAt(&now)
	value.SetCreatedAt(&now)

	err := r.orm.Create(value).Error

	if err != nil {
		logger.Errorf(err, "could not create model of type %v", modelId)
		return err
	}

	err = r.refreshAssociations(value, Create)

	if err != nil {
		logger.Errorf(err, "could not update associations of model type %v", modelId)
		return err
	}

	logger.Infof("created model of type %s with id %d", modelId, *value.GetId())

	return r.Read(ctx, value.GetId(), value)
}

func (r *repository) Read(ctx context.Context, id *uint, out ModelBased) error {
	ctx, span := r.startSubSpan(ctx, "Get")
	defer span.Finish()

	return r.orm.First(out, *id).Error
}

func (r *repository) Update(ctx context.Context, value ModelBased) error {
	modelId := r.GetModelId()
	logger := r.logger.WithContext(ctx)

	ctx, span := r.startSubSpan(ctx, "UpdateItem")
	defer span.Finish()

	now := r.clock.Now()
	value.SetUpdatedAt(&now)

	err := r.orm.Save(value).Error

	if err != nil {
		logger.Errorf(err, "could not update model of type %s with id %d", modelId, *value.GetId())
		return err
	}

	err = r.refreshAssociations(value, Update)

	if err != nil {
		logger.Errorf(err, "could not update associations of model type %s with id %d", modelId, *value.GetId())
		return err
	}

	logger.Infof("updated model of type %s with id %d", modelId, *value.GetId())

	return r.Read(ctx, value.GetId(), value)
}

func (r *repository) Delete(ctx context.Context, value ModelBased) error {
	modelId := r.GetModelId()
	logger := r.logger.WithContext(ctx)

	ctx, span := r.startSubSpan(ctx, "Delete")
	defer span.Finish()

	err := r.refreshAssociations(value, Delete)

	if err != nil {
		logger.Errorf(err, "could not delete associations of model type %s with id %d", modelId, *value.GetId())
		return err
	}

	err = r.orm.Delete(value).Error

	if err != nil {
		logger.Errorf(err, "could not delete model of type %s with id %d", modelId, *value.GetId())
	}

	logger.Infof("deleted model of type %s with id %d", modelId, *value.GetId())

	return err
}

func (r *repository) Query(ctx context.Context, qb *QueryBuilder, result interface{}) error {
	ctx, span := r.startSubSpan(ctx, "Query")
	defer span.Finish()

	db := r.orm.New()

	for _, j := range qb.joins {
		db = db.Joins(j)
	}

	if qb.where != nil {
		db = db.Where(qb.where, qb.args...)
	}

	for _, g := range qb.groupBy {
		db = db.Group(g)
	}

	for _, o := range qb.orderBy {
		db = db.Order(fmt.Sprintf("%s %s", o.field, o.direction))
	}

	if qb.page != nil {
		db = db.Offset(qb.page.offset)
		db = db.Limit(qb.page.limit)
	}

	return db.Find(result).Error
}

func (r *repository) Count(ctx context.Context, qb *QueryBuilder, model ModelBased) (int, error) {
	ctx, span := r.startSubSpan(ctx, "Count")
	defer span.Finish()

	var result = struct {
		Count int
	}{}

	db := r.orm.New()

	for _, j := range qb.joins {
		db = db.Joins(j)
	}

	if qb.where != nil {
		db = db.Where(qb.where, qb.args...)
	}

	scope := r.orm.NewScope(model)
	tableName := scope.TableName()
	key := scope.PrimaryKey()
	sel := fmt.Sprintf("COUNT(DISTINCT %s.%s) AS count", tableName, key)

	err := db.Table(tableName).Select(sel).Scan(&result).Error

	return result.Count, err
}

func (r *repository) refreshAssociations(model interface{}, op string) error {
	typeReflection := reflect.TypeOf(model).Elem()
	valueReflection := reflect.ValueOf(model).Elem()

	for i := 0; i < typeReflection.NumField(); i++ {
		field := typeReflection.Field(i)
		tag := field.Tag.Get("orm")

		if tag == "" {
			continue
		}

		tags := make(map[string]string)
		for _, tag := range strings.Split(tag, ",") {
			parts := strings.Split(tag, ":")

			value := ""
			if len(parts) == 2 {
				value = parts[1]
			}

			tags[parts[0]] = value
		}

		if _, ok := tags["assoc_update"]; !ok {
			continue
		}

		var err error

		values := valueReflection.Field(i)
		scope := r.orm.NewScope(model)
		scopeField, _ := scope.FieldByName(field.Name)

		switch op {
		case Create:
			fallthrough

		case Update:
			switch scopeField.Relationship.Kind {
			case "many_to_many":
				err = r.orm.Model(model).Association(scopeField.Name).Replace(values.Interface()).Error

			default:
				assocIds := readIdsFromReflectValue(values)
				parentId := valueReflection.FieldByName("Id").Elem().Interface()

				tableName := scopeField.DBName
				if tags["assoc_update"] != "" {
					tableName = tags["assoc_update"]
				}

				qry := fmt.Sprintf("DELETE FROM %s WHERE %s = %d", tableName, scopeField.Relationship.ForeignDBNames[0], parentId)

				if len(assocIds) != 0 {
					qry = qry + fmt.Sprintf(" AND %s NOT IN (%s)", "id", strings.Join(assocIds, ","))
				}

				err = r.orm.Exec(qry).Error
			}

		case Delete:
			switch scopeField.Relationship.Kind {
			case "has_many":
				id := valueReflection.FieldByName("Id").Elem().Interface()
				qry := fmt.Sprintf("DELETE FROM %s WHERE %s = %d", scopeField.DBName, scopeField.Relationship.ForeignDBNames[0], id)
				err = r.orm.Exec(qry).Error

			default:
				err = r.orm.Model(model).Association(field.Name).Clear().Error
			}

		default:
			err = fmt.Errorf("unkown operation")
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func (r *repository) GetModelId() string {
	return r.settings.Metadata.ModelId.String()
}

func (r *repository) GetModelName() string {
	return r.settings.Metadata.ModelId.Name
}

func (r *repository) GetMetadata() Metadata {
	return r.settings.Metadata
}

func (r *repository) startSubSpan(ctx context.Context, action string) (context.Context, tracing.Span) {
	modelName := r.GetModelId()
	spanName := fmt.Sprintf("db_repo.%v.%v", modelName, action)

	ctx, span := r.tracer.StartSubSpan(ctx, spanName)
	span.AddMetadata("model", modelName)

	return ctx, span
}

func readIdsFromReflectValue(values reflect.Value) []string {
	ids := make([]string, 0)

	for j := 0; j < values.Len(); j++ {
		id := values.Index(j).Elem().FieldByName("Id").Interface().(*uint)
		ids = append(ids, strconv.Itoa(int(*id)))
	}

	return ids
}

// UI endpoint for posting city layout validate json, and forward to ui.layout topic on RabbitMQ
//
//     Schemes: http, https
//     Host: localhost:2020
//     Version: 0.0.1
//
// swagger:meta
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gofrs/uuid"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/spf13/viper"
	"github.com/streadway/amqp"

	auth "github.com/fredericalix/yic_auth"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
		panic(fmt.Sprintf("%s: %s", msg, err))
	}
}

// Layout of a city
type Layout struct {
	// swagger:strfmt uuid
	// required: true
	ID uuid.UUID `json:"id"`
	// Name of your city
	Name string `json:"name"`

	// Grid dimension
	Grid struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"grid"`

	// List of buildings
	Buildings []struct {
		// swagger:strfmt uuid
		// required: true
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name,omitempty"`
		// required: true
		Type  string   `json:"type"`
		Roles []string `json:"roles,omitempty"`
		// required: true
		Location struct {
			X int `json:"x"`
			Y int `json:"y,"`
		} `json:"location"`
		Orientation string `json:"orientation,omitempty"`
	} `json:"buildings"`

	// 2d array of the type of road in the grid
	Roads json.RawMessage `json:"roads"`
}

type handler struct {
	store Store
	conn  *amqp.Connection
	ch    *amqp.Channel
}

func main() {
	viper.AutomaticEnv()
	viper.SetDefault("PORT", "8080")

	configFile := flag.String("config", "./config.toml", "path of the config file")
	flag.Parse()
	viper.SetConfigFile(*configFile)
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		log.Printf("cannot read config file: %v\nUse env instead\n", err)
	}

	h := handler{
		store: NewPostgreSQL(viper.GetString("POSTGRESQL_URI")),
	}

	h.conn, err = amqp.Dial(viper.GetString("RABBITMQ_URI"))
	failOnError(err, "Failed to connect to RabbitMQ")

	e := echo.New()
	e.Use(middleware.Logger())
	g := e.Group("/ui", auth.Middleware(auth.NewValidHTTP(viper.GetString("AUTH_CHECK_URI")), auth.Roles{"ui": "rw"}))
	g.GET("/_health", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	g.POST("/layout", h.postLayout)
	g.DELETE("/layout/:id", h.deleteLayout)

	go func() {
		log.Fatalf("closing: %s", <-h.conn.NotifyClose(make(chan *amqp.Error)))
	}()

	go h.handlerRPCFindLastest()

	h.ch, err = h.conn.Channel()
	failOnError(err, "Failed to open a channel")

	err = h.ch.ExchangeDeclare(
		"ui.layout", // name
		"topic",     // type
		true,        // durable
		false,       // auto-deleted
		false,       // internal
		false,       // no-wait
		nil,         // arguments
	)
	failOnError(err, "Failed to declare an exchange")

	// start the server
	host := ":" + viper.GetString("PORT")
	tlscert := viper.GetString("TLS_CERT")
	tlskey := viper.GetString("TLS_KEY")
	if tlscert == "" || tlskey == "" {
		e.Logger.Error("No cert or key provided. Start server using HTTP instead of HTTPS !")
		e.Logger.Fatal(e.Start(host))
	}
	e.Logger.Fatal(e.StartTLS(host, tlscert, tlskey))
}

// swagger:parameters authAPIKey layout
type authAPIKey struct {
	// Your yourITcity API Key (Bearer <your_sensor_token>)
	//in: header
	//required: true
	Authorization string
}

//swagger:parameters uiRequest layout
type uiRequest struct {
	//in:body
	Body Layout
}

// swagger:route POST /ui/layout layout
//
// UI Layout update
//
// Send your City layout update.
//
// Consumes:
//   application/json
// Produces:
//   application/json
// Schema:
//   http, https
// Responces:
//   200: Success
//   401: Unauthorized
//   400: Bad Request
func (h *handler) postLayout(c echo.Context) error {
	// Auth
	account := c.Get("account").(auth.Account)

	// Validate json sensor
	var cityLayout Layout
	if err := c.Bind(&cityLayout); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": err.Error()})
	}

	body, err := json.Marshal(cityLayout)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": err.Error()})
	}

	if cityLayout.ID == uuid.Nil {
		cityLayout.ID = uuid.Must(uuid.NewV4())
	}

	layout := LayoutDB{
		AccountID:  account.ID,
		LayoutID:   cityLayout.ID,
		ReceivedAt: time.Now(),
		Data:       json.RawMessage(body),
	}

	err = h.store.NewLayout(&layout)
	if err != nil {
		c.Logger().Errorf("city layout: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	err = h.ch.Publish(
		"ui.layout", // exchange
		fmt.Sprintf("%s.%s.update", account.ID, cityLayout.ID), // routing key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			Headers: amqp.Table{
				"timestamp": layout.ReceivedAt.Format(time.RFC3339Nano),
			},
			ContentType: "application/json",
			Body:        body,
		})
	if err != nil {
		c.Logger().Errorf("sending city layout: %v", err)
	}

	return c.JSON(http.StatusOK, layout)
}

// swagger:route DELETE /ui/layout/{id} deletelayout
//
// Remove an UI Layout
//
// Consumes:
//   application/json
// Schema:
//   http, https
// Responces:
//   200: Success
//   401: Unauthorized
//   404: Not Found
//   500: Internal Server Error
func (h *handler) deleteLayout(c echo.Context) error {
	// Auth
	account := c.Get("account").(auth.Account)
	lid, err := uuid.FromString(c.Param("id"))
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}
	err = h.store.DeleteLayout(account.ID, lid)
	if err != nil {
		c.Logger().Errorf("cannot delete layout %v %v: %v", account.ID, lid, err)
		return c.NoContent(http.StatusInternalServerError)
	}

	err = h.ch.Publish(
		"ui.layout", // exchange
		fmt.Sprintf("%s.%s.delete", account.ID, lid), // routing key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			Headers: amqp.Table{
				"timestamp": time.Now().Format(time.RFC3339Nano),
			},
		})
	if err != nil {
		c.Logger().Errorf("sending city layout: %v", err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *handler) handlerRPCFindLastest() {
	ch, err := h.conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"rpc_ui_latest", // name
		false,           // durable
		false,           // delete when usused
		false,           // exclusive
		false,           // no-wait
		nil,             // arguments
	)
	failOnError(err, "Failed to declare a queue")

	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	failOnError(err, "Failed to set QoS")

	msgs, err := ch.Consume(
		q.Name,        // queue
		"ui-endpoint", // consumer
		false,         // auto-ack
		false,         // exclusive
		false,         // no-local
		false,         // no-wait
		nil,           // args
	)
	failOnError(err, "Failed to register a consumer")

	log.Println("Ready, wait RPC call on queue 'rpc_ui_latest'")

	for d := range msgs {
		request := struct {
			AID uuid.UUID `json:"aid"`
		}{}
		err := json.Unmarshal(d.Body, &request)
		if err != nil {
			log.Printf("fail to unmarchal request: %v", err)
			d.Ack(false)
			continue
		}

		log.Printf("RPCfindLatest aid=%v CORRID=%v", request.AID, d.CorrelationId)

		layouts, err := h.store.FindLastByAID(request.AID)
		if err != nil {
			log.Printf("fail to find %v CORRID=%v: %v", request.AID, d.CorrelationId, err)
			d.Ack(false)
			continue
		}

		responce, err := json.Marshal(layouts)
		if err != nil {
			log.Printf("fail to marshal sensors data: %v", err)
			d.Ack(false)
			continue
		}

		err = ch.Publish(
			"",        // exchange
			d.ReplyTo, // routing key
			false,     // mandatory
			false,     // immediate
			amqp.Publishing{
				ContentType:   "application/json",
				CorrelationId: d.CorrelationId,
				Body:          responce,
			})
		if err != nil {
			log.Printf("failed to publish a message: %v", err)
		}
		d.Ack(false)
	}
}

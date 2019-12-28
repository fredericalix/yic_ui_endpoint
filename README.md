# UI API Endpoint

This is the specification of the UI API and description of different json message involve.

Each endpoint call must be authentificate with the auth API and have the right authorisation.

## Change a city layout

    curl -X POST                                                             \
       -H "Authorization: Bearer <YourToken>"                                \
       -H "Content-Type: application/json"                                   \
       https://youritcity.io/ui/layout                                       \
       -d '{"id": "43af848b-be20-49d4-bce8-17f460b19b0f" , ...}'

## Exemple of a city layout

``` json
    {
        "id": "43af848b-be20-49d4-bce8-17f460b19b0f",
        "name": "City name",

        "grid": {
            "width": 4,
            "height": 4
        },
        "buildings": [
            {
                "id": "339e0d4e-39fe-4c03-9f85-f49e3fa97e31",
                "name": "house0",
                "type": "tinyhouse1",
                "roles": [ "server", "nginx", "http front" ],
                "location": {
                    "x": 1,
                    "y": 1
                }
            },
            {
                "id": "4dbc6560-3123-4d6a-aa52-fb22074b8691",
                "name": "house1",
                "type": "tinyhouse1",
                "roles": [ "server", "nginx", "http front" ],
                "location": {
                    "x": 1,
                    "y": 3
                }
            },
        ],
        "moving_objects": [
            {
                "to": "define"
            },
        ]
    }
```

With a change in any part of this layout, the entire layout must be POST.

## Compile & run

    go build
    ./ui-endpoint

## Exemple config in Environment variables

``` sh
    RABBITMQ_URI="amqp://guest:guest@rabbit:5672"
    AUTH_CHECK_URI="http://auth:1234/auth/check"
    POSTGRESQL_URI="postgresql://postgres:passwords@localhost:5432/yic_ui?sslmode=disable"
    PORT=8080
```

## Generate of the swagger doc

Install go-swagger <https://goswagger.io/install.html> then generate the swagger specification

    swagger generate spec -o swagger_ui.json

To quick show the doc

    swagger serve swagger_ui.json

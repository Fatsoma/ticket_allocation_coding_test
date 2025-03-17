# Ticket Allocation Coding Test

## Introduction

Thanks for applying for a development role at Fatsoma. To give us a good
indication of programming ability and style please submit your solution for
this ticket allocation problem.

This is not a timed test, but you do not need to spend more than a couple of
hours on this. A partial solution is still very useful, and you can describe
your thoughts for next steps that would be taken.

Your submission must be your own work.

### Languages and frameworks

For reference, here at Fatsoma we primarily develop using ruby on rails and
golang. However feel free to solve this in whatever language you feel comfortable with and use a framework if desired.

We are mainly looking for clean, well architected, tested code that highlights
your skill set and shows technical proficiency.

### Database

Included is an SQL dump from PostgreSQL. It is not required to use this but
should be helpful. You may need to amend this to add constraints.

PostgreSQL has all the functionality required for satisfying this problem set,
with some features introduced version 9.5 that may be of interest. You may
choose to use a different database engine that satisfies the requirements of
this problem.

---

## Problem definition

The following three routes need to be built to enable allocating of ticket
options to multiple purchases. They all correspond to the jsonapi spec (https://jsonapi.org/)

The solution needs to ensure that the allocation does not drop below 0,
and the purchased amounts are not greater than the allocation given.

Expect multiple requests to be made against this API concurrently.

We use the term purchase but taking payment is out of scope for this problem.

## Routes with Example Requests

The Swagger definition and postman collection are also available in this repository for reference.

### Create Ticket Option

Create a ticket_option with an allocation of tickets available to purchase:

`POST /ticket_options`

Request Body:

```json
{
    "data": {
        "type": "ticket_options",
        "attributes": {
            "name": "example",
            "description": "sample description",
            "allocation": 100
        }
    }
}
```

Response Body:

```json
{
    "data": {
        "type": "ticket_options",
        "id": "70b751fe-04dd-4dd1-8955-ab9b188ddb1f",
        "attributes": {
            "name": "example",
            "description": "sample description",
            "allocation": 100
        }
    }
}
```

### Get Ticket Option

Get ticket option by id:

`GET /ticket_options/:id`

(No request body)

Response Body:

```json
{
    "data": {
        "type": "ticket_options",
        "id": "70b751fe-04dd-4dd1-8955-ab9b188ddb1f",
        "attributes": {
            "name": "example",
            "description": "sample description",
            "allocation": 100
        }
    }
}
```

### Purchase from Ticket Option

Purchase a quantity of tickets from the allocation of the given ticket_option:

`POST /purchases`

Request body:

```json
{
    "data": {
        "type": "purchases",
        "attributes": {
            "quantity": 2
        },
        "relationships": {
            "ticket_option": {
                "data": {
                    "type": "ticket_options",
                    "id": "969f4317-09f4-4b15-b8be-a87d40fb56fb"
                }
            },
            "user": {
                "data": {
                    "type": "users",
                    "id": "d6abe829-c28c-44ec-bee6-3183f2c53fef"
                }
            }
        }
    }
}

```

Response Body:

A 2xx status code must be returned on success.

```json
{
    "data": {
        "type": "purchases",
        "id": "cd837712-fd86-4345-9e7f-d519c8db6c45",
        "attributes": {
            "quantity": 2
        },
        "relationships": {
            "ticket_option": {
                "data": {
                    "type": "ticket_options",
                    "id": "969f4317-09f4-4b15-b8be-a87d40fb56fb"
                }
            },
            "user": {
                "data": {
                    "type": "users",
                    "id": "d6abe829-c28c-44ec-bee6-3183f2c53fef"
                }
            }
        }
    }
}
```

A 4xx status code must be returned on any request that attempts to purchase more tickets than are available. In this case, no tickets should be purchased for that request. Example error response given below

```json
{
    "errors": [
        {
            "status": "400",
            "code": "invalid_purchase_quantity",
            "title": "Unable to purchase provided quantity",
            "detail": "Unable to reserve given quantity of ticket options",
            "source": {
                "pointer": "/data/quantity"
            }
        }
    ]
}
```

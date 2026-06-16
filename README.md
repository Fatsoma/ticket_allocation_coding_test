# Ticket Allocation Coding Test

## Introduction

Thanks for applying for a development role at Fatsoma. To give us a good
indication of your programming ability and style, please submit your solution
for this ticket allocation problem. We are mainly looking for clean, well
architected, tested code that highlights your skill set and shows technical
proficiency.

This is not a timed test, but you do not need to spend more than a couple of
hours on it. We have tried to keep the scope small so that time is spent on the
interesting parts rather than on boilerplate. A partial solution is still very
useful — if you run out of time, describe in your submission what you would do
next.

Your submission must be your own work.

### Use of AI tools

Using AI tools (e.g. GitHub Copilot, ChatGPT, Claude) is allowed — they are part
of how we work day to day. However, please **declare any AI co-authorship** in
your submission notes: briefly note where and how you used AI (for example,
scaffolding, generating tests, or rubber-ducking the locking approach). You
remain responsible for everything you submit, so make sure you understand and
can explain your solution — we will likely discuss it when we meet.


### Languages and frameworks

For reference, here at Fatsoma we primarily develop using Ruby on Rails and
Go. However, feel free to solve this in whatever language you are most
comfortable with, and use a framework if you wish. We are interested in how you
structure and reason about the solution, not in any particular stack.

## Problem definition

The following three routes need to be built to enable allocating ticket options
to multiple purchases. They all correspond to the [JSON:API spec](https://jsonapi.org/).
If helpful, a list of JSON:API implementation libraries across various languages
is available at https://jsonapi.org/implementations/. We're not assessing strict
conformance to the spec, so minor differences in the output produced by your
chosen library are fine.

A `ticket_option` is created with a fixed `allocation` — the total number of
tickets available to purchase. Each `purchase` draws a `quantity` of tickets
against a `ticket_option`.

The solution must guarantee the following invariant at all times:

> The sum of purchased quantities for a `ticket_option` must never exceed its
> `allocation`, and the remaining availability must never drop below 0.

The `allocation` value on a `ticket_option` represents its **total** capacity
and does not change once created; remaining availability is derived from the
allocation minus the quantities already purchased. The `GET` route returns the
`ticket_option` as created (i.e. the original `allocation`).

Expect requests to be made against this API concurrently, and design it to scale
horizontally: multiple instances of the program may run at once to handle large
numbers of requests. The invariant above must hold regardless of how many
requests, or how many running instances, are involved — including when many
purchases hit the same `ticket_option` at the same time.

We use the term "purchase", but taking payment is out of scope. You do not need
to build authentication, payment handling, a UI, or anything beyond the three
routes described below.

### Database

Included is an SQL dump from PostgreSQL (`database_structure.sql`). You are not
required to use it, but it should be a helpful starting point. Note that it
intentionally ships **without** integrity constraints (no foreign keys, nullable
columns, no `CHECK`s) — part of the exercise is deciding what constraints the
schema should have.

The SQL dump is provided purely as an example. Whatever storage you use, it must
let you prevent overselling a `ticket_option`'s allocation under concurrent load
across multiple instances. Beyond that, you may use any database engine or
storage approach you like.

## Routes with example requests

All requests and responses use the JSON:API media type
`application/vnd.api+json`. The Swagger definition
(`Ticket_Allocation.swagger.yaml`) and Postman collection
(`Ticket_Allocation.postman_collection.json`) are also available in this
repository for reference.

### Create ticket option

Create a `ticket_option` with an allocation of tickets available to purchase:

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

Response Body (`201 Created`):

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

`allocation` must be a non-negative integer. Requests with a missing `name` or a
negative/non-integer `allocation` should return a `4xx` error.

### Get ticket option

Get ticket option by id:

`GET /ticket_options/:id`

(No request body)

Response Body (`200 OK`):

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

A request for an id that does not exist should return a `404 Not Found`.

### Purchase from ticket option

Purchase a quantity of tickets from the allocation of the given `ticket_option`
and associate it with a user (N.B. managing a user resource is not being looked
at here, so an example id is sufficient):

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

A `2xx` status code must be returned on success.

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

A `4xx` status code must be returned on any request that attempts to purchase
more tickets than are available. In this case, no tickets should be purchased
for that request. Example error response given below:

```json
{
    "errors": [
        {
            "status": "400",
            "code": "invalid_purchase_quantity",
            "title": "Unable to purchase provided quantity",
            "detail": "Unable to reserve given quantity of ticket options",
            "source": {
                "pointer": "/data/attributes/quantity"
            }
        }
    ]
}
```

### Validation and error handling

Please handle at least the following cases with an appropriate `4xx` status and
a JSON:API `errors` payload (the shape above is a good template):

- `quantity` that is zero or negative.
- A purchase referencing a `ticket_option` id that does not exist
  (`404 Not Found`).
- A request that would exceed the remaining allocation (`code:
  invalid_purchase_quantity`, as above) — no tickets persisted.
- Missing required attributes on create (e.g. `name`, `allocation`).

You do not need to exhaustively validate every field; we are more interested in
seeing a consistent, sensible approach than full coverage.

## Submitting your solution

Please include a short README in your submission covering:

1. **How to run it** — setup steps, dependencies, and how to start the API
   (including any database setup/migrations).
2. **How to run the tests.**
3. **Notes** — any assumptions you made, trade-offs, what you would do next
   given more time, and a short declaration of any AI tool usage (see
   [Use of AI tools](#use-of-ai-tools)).

Submit your solution as a link to a public Git repository (e.g. GitHub) or as a
zip/tarball, whichever is easier for you. Please do not include compiled
artifacts or dependency directories (e.g. `node_modules`, `vendor`).

If anything in this brief is unclear, make a reasonable assumption, state it in
your notes, and carry on — we are happy to discuss your reasoning when we meet.

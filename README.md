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
your skillset and shows technical proficiency.

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
options to multiple purchases.

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
  "name": "example",
  "desc": "sample description",
  "allocation": 100
}
```

Response Body:

```json
{
  "id": "70b751fe-04dd-4dd1-8955-ab9b188ddb1f",
  "name": "example",
  "desc": "sample description",
  "allocation": 100
}
```

### Get Ticket Option

Get ticket option by id:

`GET /ticket_options/:id`

(No request body)

Response Body:

```json
{
  "id": "70b751fe-04dd-4dd1-8955-ab9b188ddb1f",
  "name": "example",
  "desc": "sample description",
  "allocation": 100
}
```

### Purchase from Ticket Option

Purchase a quantity of tickets from the allocation of the given ticket_option:

`POST /ticket_options/:id/purchases`

Request body:

```json
{
  "quantity": 2,
  "user_id": "406c1d05-bbb2-4e94-b183-7d208c2692e1"
}
```

(No Response body)

A 2xx status code must be returned on success.

A 4xx status code must be returned on any request that attempts to purchase more tickets than are available. In this case, no tickets should be purchased for that request.

## License

See LICENSE file.

Please be honest and do not share your solutions to this challenge with other job applicants. You may adapt this problem for your own company's hiring process but it should be changed enough to require applicants to do their own work.

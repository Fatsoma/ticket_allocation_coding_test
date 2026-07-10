-- Ticket allocation schema with integrity constraints.
-- The purchased counter is denormalised for O(1) atomic reservation;
-- CHECK (purchased <= allocation) is the belt-and-braces backstop.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE TABLE public.ticket_options (
    id uuid DEFAULT public.uuid_generate_v4() PRIMARY KEY,
    name character varying NOT NULL,
    description character varying NOT NULL DEFAULT '',
    allocation integer NOT NULL CHECK (allocation >= 0),
    purchased integer NOT NULL DEFAULT 0,
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    updated_at timestamp without time zone NOT NULL DEFAULT now(),
    CONSTRAINT ticket_options_purchased_bounds
        CHECK (purchased >= 0 AND purchased <= allocation)
);

CREATE TABLE public.purchases (
    id uuid DEFAULT public.uuid_generate_v4() PRIMARY KEY,
    quantity integer NOT NULL CHECK (quantity > 0),
    user_id uuid NOT NULL,
    ticket_option_id uuid NOT NULL REFERENCES public.ticket_options (id),
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    updated_at timestamp without time zone NOT NULL DEFAULT now()
);

CREATE INDEX purchases_ticket_option_id_idx ON public.purchases (ticket_option_id);

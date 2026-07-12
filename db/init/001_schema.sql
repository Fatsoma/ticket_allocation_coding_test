-- Ticket allocation schema with bucketed capacity for write sharding.
-- Capacity is split across ticket_option_buckets at create time.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE TABLE public.ticket_options (
    id uuid DEFAULT public.uuid_generate_v4() PRIMARY KEY,
    name character varying NOT NULL,
    description character varying NOT NULL DEFAULT '',
    allocation integer NOT NULL CHECK (allocation >= 1),
    bucket_count integer NOT NULL DEFAULT 1 CHECK (bucket_count >= 1 AND bucket_count <= 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE public.ticket_option_buckets (
    id uuid DEFAULT public.uuid_generate_v4() PRIMARY KEY,
    ticket_option_id uuid NOT NULL REFERENCES public.ticket_options (id),
    bucket_index integer NOT NULL,
    capacity integer NOT NULL CHECK (capacity >= 1),
    purchased integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (ticket_option_id, bucket_index),
    CONSTRAINT ticket_option_buckets_purchased_bounds
        CHECK (purchased >= 0 AND purchased <= capacity)
);

CREATE TABLE public.purchases (
    id uuid DEFAULT public.uuid_generate_v4() PRIMARY KEY,
    quantity integer NOT NULL CHECK (quantity > 0),
    user_id uuid NOT NULL,
    ticket_option_id uuid NOT NULL REFERENCES public.ticket_options (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE public.purchase_allocations (
    purchase_id uuid NOT NULL REFERENCES public.purchases (id),
    bucket_id uuid NOT NULL REFERENCES public.ticket_option_buckets (id),
    quantity integer NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (purchase_id, bucket_id)
);

CREATE INDEX purchases_ticket_option_id_idx ON public.purchases (ticket_option_id);
CREATE INDEX ticket_option_buckets_ticket_option_id_idx ON public.ticket_option_buckets (ticket_option_id);

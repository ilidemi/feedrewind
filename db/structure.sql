--
-- PostgreSQL database dump
--

-- Dumped from database version 12.5 (Ubuntu 12.5-0ubuntu0.20.04.1)
-- Dumped by pg_dump version 14.10

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


--
-- Name: billing_interval; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.billing_interval AS ENUM (
    'monthly',
    'yearly'
);


--
-- Name: blog_crawl_vote_value; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.blog_crawl_vote_value AS ENUM (
    'confirmed',
    'looks_wrong'
);


--
-- Name: blog_post_category_top_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.blog_post_category_top_status AS ENUM (
    'top_only',
    'top_and_custom',
    'custom_only'
);


--
-- Name: blog_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.blog_status AS ENUM (
    'crawl_in_progress',
    'crawl_failed',
    'crawled_voting',
    'crawled_confirmed',
    'crawled_looks_wrong',
    'manually_inserted',
    'update_from_feed_failed'
);


--
-- Name: blog_update_action; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.blog_update_action AS ENUM (
    'recrawl',
    'update_from_feed_or_fail',
    'no_op',
    'fail'
);


--
-- Name: day_of_week; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.day_of_week AS ENUM (
    'mon',
    'tue',
    'wed',
    'thu',
    'fri',
    'sat',
    'sun'
);


--
-- Name: old_blog_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.old_blog_status AS ENUM (
    'crawl_in_progress',
    'crawled',
    'confirmed',
    'live',
    'crawl_failed',
    'crawled_looks_wrong'
);


--
-- Name: post_delivery_channel; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.post_delivery_channel AS ENUM (
    'single_feed',
    'multiple_feeds',
    'email'
);


--
-- Name: post_publish_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.post_publish_status AS ENUM (
    'rss_published',
    'email_pending',
    'email_skipped',
    'email_sent'
);


--
-- Name: postmark_message_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.postmark_message_type AS ENUM (
    'sub_initial',
    'sub_final',
    'sub_post'
);


--
-- Name: subscription_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.subscription_status AS ENUM (
    'waiting_for_blog',
    'setup',
    'live',
    'custom_blog_requested'
);


--
-- Name: bump_updated_at_utc(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.bump_updated_at_utc() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
begin
	NEW.updated_at = utc_now();
	return NEW;
end;
$$;


--
-- Name: utc_now(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.utc_now() RETURNS timestamp without time zone
    LANGUAGE plpgsql
    AS $$
begin
	return (now() at time zone 'utc');
end;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: admin_telemetries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_telemetries (
    id bigint NOT NULL,
    key text NOT NULL,
    value double precision NOT NULL,
    extra json,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: admin_telemetries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.admin_telemetries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: admin_telemetries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.admin_telemetries_id_seq OWNED BY public.admin_telemetries.id;


--
-- Name: ar_internal_metadata; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ar_internal_metadata (
    key character varying NOT NULL,
    value character varying,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: blog_canonical_equality_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_canonical_equality_configs (
    blog_id bigint NOT NULL,
    same_hosts text[],
    expect_tumblr_paths boolean NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: blog_crawl_client_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_crawl_client_tokens (
    value character varying NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    blog_id bigint NOT NULL
);


--
-- Name: blog_crawl_progresses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_crawl_progresses (
    progress character varying,
    count integer,
    epoch integer NOT NULL,
    epoch_times character varying,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    blog_id bigint NOT NULL
);


--
-- Name: blog_crawl_votes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_crawl_votes (
    id bigint NOT NULL,
    blog_id bigint NOT NULL,
    value public.blog_crawl_vote_value NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    user_id bigint
);


--
-- Name: blog_crawl_votes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.blog_crawl_votes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: blog_crawl_votes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.blog_crawl_votes_id_seq OWNED BY public.blog_crawl_votes.id;


--
-- Name: blog_discarded_feed_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_discarded_feed_entries (
    id bigint NOT NULL,
    blog_id bigint NOT NULL,
    url character varying NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: blog_discarded_feed_entries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.blog_discarded_feed_entries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: blog_discarded_feed_entries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.blog_discarded_feed_entries_id_seq OWNED BY public.blog_discarded_feed_entries.id;


--
-- Name: blog_missing_from_feed_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_missing_from_feed_entries (
    id bigint NOT NULL,
    blog_id bigint NOT NULL,
    url text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: blog_missing_from_feed_entries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.blog_missing_from_feed_entries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: blog_missing_from_feed_entries_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.blog_missing_from_feed_entries_id_seq OWNED BY public.blog_missing_from_feed_entries.id;


--
-- Name: blog_post_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_post_categories (
    id bigint NOT NULL,
    blog_id bigint NOT NULL,
    name text NOT NULL,
    index integer NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    top_status public.blog_post_category_top_status NOT NULL
);


--
-- Name: blog_post_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.blog_post_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: blog_post_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.blog_post_categories_id_seq OWNED BY public.blog_post_categories.id;


--
-- Name: blog_post_category_assignments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_post_category_assignments (
    id bigint NOT NULL,
    blog_post_id bigint NOT NULL,
    category_id bigint NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: blog_post_category_assignments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.blog_post_category_assignments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: blog_post_category_assignments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.blog_post_category_assignments_id_seq OWNED BY public.blog_post_category_assignments.id;


--
-- Name: blog_post_locks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_post_locks (
    blog_id bigint NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: blog_posts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blog_posts (
    id bigint NOT NULL,
    blog_id bigint NOT NULL,
    index integer NOT NULL,
    url character varying NOT NULL,
    title character varying NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: blog_posts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.blog_posts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: blog_posts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.blog_posts_id_seq OWNED BY public.blog_posts.id;


--
-- Name: blogs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blogs (
    id bigint NOT NULL,
    name character varying NOT NULL,
    feed_url character varying NOT NULL,
    status public.blog_status NOT NULL,
    status_updated_at timestamp without time zone NOT NULL,
    version integer NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    update_action public.blog_update_action NOT NULL,
    url character varying,
    start_feed_id bigint
);


--
-- Name: custom_blog_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_blog_requests (
    id bigint NOT NULL,
    blog_url text,
    feed_url text NOT NULL,
    stripe_payment_intent_id text,
    user_id bigint,
    subscription_id bigint,
    blog_id bigint,
    fulfilled_at timestamp without time zone,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    why text NOT NULL,
    enable_for_others boolean NOT NULL
);


--
-- Name: COLUMN custom_blog_requests.subscription_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.custom_blog_requests.subscription_id IS 'Protects unfulfilled subscriptions from accidental deletion. Set to null when a request is fulfilled.';


--
-- Name: custom_blog_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.custom_blog_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: custom_blog_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.custom_blog_requests_id_seq OWNED BY public.custom_blog_requests.id;


--
-- Name: delayed_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.delayed_jobs (
    id bigint NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    handler text NOT NULL,
    last_error text,
    run_at timestamp without time zone,
    locked_at timestamp without time zone,
    failed_at timestamp without time zone,
    locked_by character varying,
    queue character varying,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: delayed_jobs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.delayed_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: delayed_jobs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.delayed_jobs_id_seq OWNED BY public.delayed_jobs.id;


--
-- Name: ignored_suggestion_feeds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ignored_suggestion_feeds (
    feed_url text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: patron_credits; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.patron_credits (
    user_id bigint NOT NULL,
    count integer NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    CONSTRAINT count_non_negative CHECK ((count >= 0))
);


--
-- Name: patron_invoices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.patron_invoices (
    id text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: postmark_bounced_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.postmark_bounced_users (
    user_id bigint NOT NULL,
    example_bounce_id bigint NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: postmark_bounces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.postmark_bounces (
    id bigint NOT NULL,
    bounce_type text NOT NULL,
    message_id text,
    payload json NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: postmark_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.postmark_messages (
    message_id text NOT NULL,
    subscription_post_id bigint,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    message_type public.postmark_message_type NOT NULL,
    subscription_id bigint NOT NULL
);


--
-- Name: pricing_offers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pricing_offers (
    id text NOT NULL,
    monthly_rate money NOT NULL,
    yearly_rate money NOT NULL,
    stripe_product_id text,
    stripe_monthly_price_id text,
    stripe_yearly_price_id text,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    plan_id text NOT NULL
);


--
-- Name: pricing_plans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pricing_plans (
    id text NOT NULL,
    default_offer_id text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: product_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_events (
    id bigint NOT NULL,
    event_type text NOT NULL,
    event_properties json,
    user_properties json,
    user_ip text,
    dispatched_at timestamp without time zone,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    product_user_id text NOT NULL,
    browser text,
    os_name text,
    os_version text,
    bot_name text
);


--
-- Name: product_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.product_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: product_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.product_events_id_seq OWNED BY public.product_events.id;


--
-- Name: schedules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schedules (
    id bigint NOT NULL,
    day_of_week public.day_of_week NOT NULL,
    count integer NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    subscription_id bigint NOT NULL
);


--
-- Name: schedules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.schedules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: schedules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.schedules_id_seq OWNED BY public.schedules.id;


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version character varying NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: start_feeds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.start_feeds (
    id bigint NOT NULL,
    content bytea,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    url text NOT NULL,
    final_url text,
    title text NOT NULL,
    start_page_id integer
);


--
-- Name: start_pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.start_pages (
    id bigint NOT NULL,
    content bytea NOT NULL,
    url text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    final_url text NOT NULL
);


--
-- Name: start_pages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.start_pages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: start_pages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.start_pages_id_seq OWNED BY public.start_pages.id;


--
-- Name: stripe_hanging_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stripe_hanging_sessions (
    stripe_session_id text NOT NULL,
    stripe_subscription_id text NOT NULL,
    stripe_customer_id text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: stripe_subscription_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stripe_subscription_tokens (
    id bigint NOT NULL,
    offer_id text NOT NULL,
    stripe_subscription_id text NOT NULL,
    stripe_customer_id text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    billing_interval public.billing_interval NOT NULL,
    current_period_end timestamp without time zone NOT NULL
);


--
-- Name: stripe_webhook_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.stripe_webhook_events (
    id text NOT NULL,
    payload bytea NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: subscription_posts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscription_posts (
    id bigint NOT NULL,
    blog_post_id bigint NOT NULL,
    subscription_id bigint NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    published_at timestamp without time zone,
    published_at_local_date character varying,
    publish_status public.post_publish_status,
    random_id text NOT NULL
);


--
-- Name: subscription_posts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.subscription_posts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: subscription_posts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.subscription_posts_id_seq OWNED BY public.subscription_posts.id;


--
-- Name: subscription_rsses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscription_rsses (
    id bigint NOT NULL,
    body text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    subscription_id bigint NOT NULL
);


--
-- Name: subscription_rsses_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.subscription_rsses_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: subscription_rsses_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.subscription_rsses_id_seq OWNED BY public.subscription_rsses.id;


--
-- Name: subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscriptions (
    id bigint NOT NULL,
    blog_id bigint NOT NULL,
    name character varying NOT NULL,
    status public.subscription_status NOT NULL,
    is_paused boolean NOT NULL,
    is_added_past_midnight boolean,
    discarded_at timestamp without time zone,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    finished_setup_at timestamp without time zone,
    final_item_published_at timestamp without time zone,
    user_id bigint,
    initial_item_publish_status public.post_publish_status,
    final_item_publish_status public.post_publish_status,
    schedule_version integer NOT NULL,
    anon_product_user_id uuid,
    CONSTRAINT subscriptions_refers_to_user CHECK ((NOT ((user_id IS NULL) AND (anon_product_user_id IS NULL))))
);


--
-- Name: subscriptions_with_discarded; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.subscriptions_with_discarded AS
 SELECT subscriptions.id,
    subscriptions.blog_id,
    subscriptions.name,
    subscriptions.status,
    subscriptions.is_paused,
    subscriptions.is_added_past_midnight,
    subscriptions.discarded_at,
    subscriptions.created_at,
    subscriptions.updated_at,
    subscriptions.finished_setup_at,
    subscriptions.final_item_published_at,
    subscriptions.user_id,
    subscriptions.initial_item_publish_status,
    subscriptions.final_item_publish_status,
    subscriptions.schedule_version,
    subscriptions.anon_product_user_id
   FROM public.subscriptions
  WITH CASCADED CHECK OPTION;


--
-- Name: subscriptions_without_discarded; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.subscriptions_without_discarded AS
 SELECT subscriptions.id,
    subscriptions.blog_id,
    subscriptions.name,
    subscriptions.status,
    subscriptions.is_paused,
    subscriptions.is_added_past_midnight,
    subscriptions.discarded_at,
    subscriptions.created_at,
    subscriptions.updated_at,
    subscriptions.finished_setup_at,
    subscriptions.final_item_published_at,
    subscriptions.user_id,
    subscriptions.initial_item_publish_status,
    subscriptions.final_item_publish_status,
    subscriptions.schedule_version,
    subscriptions.anon_product_user_id
   FROM public.subscriptions
  WHERE (subscriptions.discarded_at IS NULL)
  WITH CASCADED CHECK OPTION;


--
-- Name: test_singletons; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.test_singletons (
    key text NOT NULL,
    value text,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: typed_blog_urls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.typed_blog_urls (
    id bigint NOT NULL,
    typed_url text NOT NULL,
    stripped_url text NOT NULL,
    source text NOT NULL,
    result text NOT NULL,
    user_id bigint,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
);


--
-- Name: typed_blog_urls_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.typed_blog_urls_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: typed_blog_urls_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.typed_blog_urls_id_seq OWNED BY public.typed_blog_urls.id;


--
-- Name: user_rsses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_rsses (
    body text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    user_id bigint NOT NULL
);


--
-- Name: user_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_settings (
    user_id bigint NOT NULL,
    timezone character varying NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    version integer NOT NULL,
    delivery_channel public.post_delivery_channel
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    email character varying NOT NULL,
    password_digest character varying NOT NULL,
    created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL,
    auth_token character varying NOT NULL,
    id bigint NOT NULL,
    name text NOT NULL,
    product_user_id uuid NOT NULL,
    discarded_at timestamp without time zone,
    offer_id text NOT NULL,
    stripe_subscription_id text,
    stripe_customer_id text,
    billing_interval public.billing_interval,
    stripe_cancel_at timestamp without time zone,
    stripe_current_period_end timestamp without time zone
);


--
-- Name: users_with_discarded; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.users_with_discarded AS
 SELECT users.email,
    users.password_digest,
    users.created_at,
    users.updated_at,
    users.auth_token,
    users.id,
    users.name,
    users.product_user_id,
    users.discarded_at,
    users.offer_id,
    users.stripe_subscription_id,
    users.stripe_customer_id,
    users.billing_interval,
    users.stripe_cancel_at,
    users.stripe_current_period_end
   FROM public.users
  WITH CASCADED CHECK OPTION;


--
-- Name: users_without_discarded; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.users_without_discarded AS
 SELECT users.email,
    users.password_digest,
    users.created_at,
    users.updated_at,
    users.auth_token,
    users.id,
    users.name,
    users.product_user_id,
    users.discarded_at,
    users.offer_id,
    users.stripe_subscription_id,
    users.stripe_customer_id,
    users.billing_interval,
    users.stripe_cancel_at,
    users.stripe_current_period_end
   FROM public.users
  WHERE (users.discarded_at IS NULL)
  WITH CASCADED CHECK OPTION;


--
-- Name: admin_telemetries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_telemetries ALTER COLUMN id SET DEFAULT nextval('public.admin_telemetries_id_seq'::regclass);


--
-- Name: blog_crawl_votes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_crawl_votes ALTER COLUMN id SET DEFAULT nextval('public.blog_crawl_votes_id_seq'::regclass);


--
-- Name: blog_discarded_feed_entries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_discarded_feed_entries ALTER COLUMN id SET DEFAULT nextval('public.blog_discarded_feed_entries_id_seq'::regclass);


--
-- Name: blog_missing_from_feed_entries id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_missing_from_feed_entries ALTER COLUMN id SET DEFAULT nextval('public.blog_missing_from_feed_entries_id_seq'::regclass);


--
-- Name: blog_post_categories id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_post_categories ALTER COLUMN id SET DEFAULT nextval('public.blog_post_categories_id_seq'::regclass);


--
-- Name: blog_post_category_assignments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_post_category_assignments ALTER COLUMN id SET DEFAULT nextval('public.blog_post_category_assignments_id_seq'::regclass);


--
-- Name: blog_posts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_posts ALTER COLUMN id SET DEFAULT nextval('public.blog_posts_id_seq'::regclass);


--
-- Name: custom_blog_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_blog_requests ALTER COLUMN id SET DEFAULT nextval('public.custom_blog_requests_id_seq'::regclass);


--
-- Name: delayed_jobs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delayed_jobs ALTER COLUMN id SET DEFAULT nextval('public.delayed_jobs_id_seq'::regclass);


--
-- Name: product_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_events ALTER COLUMN id SET DEFAULT nextval('public.product_events_id_seq'::regclass);


--
-- Name: schedules id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedules ALTER COLUMN id SET DEFAULT nextval('public.schedules_id_seq'::regclass);


--
-- Name: start_pages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.start_pages ALTER COLUMN id SET DEFAULT nextval('public.start_pages_id_seq'::regclass);


--
-- Name: subscription_posts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_posts ALTER COLUMN id SET DEFAULT nextval('public.subscription_posts_id_seq'::regclass);


--
-- Name: subscription_rsses id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_rsses ALTER COLUMN id SET DEFAULT nextval('public.subscription_rsses_id_seq'::regclass);


--
-- Name: typed_blog_urls id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.typed_blog_urls ALTER COLUMN id SET DEFAULT nextval('public.typed_blog_urls_id_seq'::regclass);


--
-- Name: admin_telemetries admin_telemetries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_telemetries
    ADD CONSTRAINT admin_telemetries_pkey PRIMARY KEY (id);


--
-- Name: ar_internal_metadata ar_internal_metadata_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ar_internal_metadata
    ADD CONSTRAINT ar_internal_metadata_pkey PRIMARY KEY (key);


--
-- Name: blog_canonical_equality_configs blog_canonical_equality_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_canonical_equality_configs
    ADD CONSTRAINT blog_canonical_equality_configs_pkey PRIMARY KEY (blog_id);


--
-- Name: blog_crawl_client_tokens blog_crawl_client_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_crawl_client_tokens
    ADD CONSTRAINT blog_crawl_client_tokens_pkey PRIMARY KEY (blog_id);


--
-- Name: blog_crawl_progresses blog_crawl_progresses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_crawl_progresses
    ADD CONSTRAINT blog_crawl_progresses_pkey PRIMARY KEY (blog_id);


--
-- Name: blog_crawl_votes blog_crawl_votes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_crawl_votes
    ADD CONSTRAINT blog_crawl_votes_pkey PRIMARY KEY (id);


--
-- Name: blog_discarded_feed_entries blog_discarded_feed_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_discarded_feed_entries
    ADD CONSTRAINT blog_discarded_feed_entries_pkey PRIMARY KEY (id);


--
-- Name: blog_missing_from_feed_entries blog_missing_from_feed_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_missing_from_feed_entries
    ADD CONSTRAINT blog_missing_from_feed_entries_pkey PRIMARY KEY (id);


--
-- Name: blog_post_categories blog_post_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_post_categories
    ADD CONSTRAINT blog_post_categories_pkey PRIMARY KEY (id);


--
-- Name: blog_post_category_assignments blog_post_category_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_post_category_assignments
    ADD CONSTRAINT blog_post_category_assignments_pkey PRIMARY KEY (id);


--
-- Name: blog_post_locks blog_post_locks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_post_locks
    ADD CONSTRAINT blog_post_locks_pkey PRIMARY KEY (blog_id);


--
-- Name: blog_posts blog_posts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_posts
    ADD CONSTRAINT blog_posts_pkey PRIMARY KEY (id);


--
-- Name: blogs blogs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blogs
    ADD CONSTRAINT blogs_pkey PRIMARY KEY (id);


--
-- Name: custom_blog_requests custom_blog_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_blog_requests
    ADD CONSTRAINT custom_blog_requests_pkey PRIMARY KEY (id);


--
-- Name: delayed_jobs delayed_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delayed_jobs
    ADD CONSTRAINT delayed_jobs_pkey PRIMARY KEY (id);


--
-- Name: ignored_suggestion_feeds ignored_suggestion_feeds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ignored_suggestion_feeds
    ADD CONSTRAINT ignored_suggestion_feeds_pkey PRIMARY KEY (feed_url);


--
-- Name: patron_credits patron_credits_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.patron_credits
    ADD CONSTRAINT patron_credits_pkey PRIMARY KEY (user_id);


--
-- Name: patron_invoices patron_invoices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.patron_invoices
    ADD CONSTRAINT patron_invoices_pkey PRIMARY KEY (id);


--
-- Name: postmark_bounced_users postmark_bounced_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.postmark_bounced_users
    ADD CONSTRAINT postmark_bounced_users_pkey PRIMARY KEY (user_id);


--
-- Name: postmark_bounces postmark_bounces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.postmark_bounces
    ADD CONSTRAINT postmark_bounces_pkey PRIMARY KEY (id);


--
-- Name: postmark_messages postmark_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.postmark_messages
    ADD CONSTRAINT postmark_messages_pkey PRIMARY KEY (message_id);


--
-- Name: pricing_offers pricing_offers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pricing_offers
    ADD CONSTRAINT pricing_offers_pkey PRIMARY KEY (id);


--
-- Name: pricing_plans pricing_plans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pricing_plans
    ADD CONSTRAINT pricing_plans_pkey PRIMARY KEY (id);


--
-- Name: product_events product_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_events
    ADD CONSTRAINT product_events_pkey PRIMARY KEY (id);


--
-- Name: schedules schedules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedules
    ADD CONSTRAINT schedules_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: start_feeds start_feeds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.start_feeds
    ADD CONSTRAINT start_feeds_pkey PRIMARY KEY (id);


--
-- Name: start_pages start_pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.start_pages
    ADD CONSTRAINT start_pages_pkey PRIMARY KEY (id);


--
-- Name: stripe_hanging_sessions stripe_hanging_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stripe_hanging_sessions
    ADD CONSTRAINT stripe_hanging_sessions_pkey PRIMARY KEY (stripe_session_id);


--
-- Name: stripe_subscription_tokens stripe_subscription_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stripe_subscription_tokens
    ADD CONSTRAINT stripe_subscription_tokens_pkey PRIMARY KEY (id);


--
-- Name: stripe_webhook_events stripe_webhook_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stripe_webhook_events
    ADD CONSTRAINT stripe_webhook_events_pkey PRIMARY KEY (id);


--
-- Name: subscription_posts subscription_posts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_posts
    ADD CONSTRAINT subscription_posts_pkey PRIMARY KEY (id);


--
-- Name: subscription_rsses subscription_rsses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_rsses
    ADD CONSTRAINT subscription_rsses_pkey PRIMARY KEY (id);


--
-- Name: subscription_rsses subscription_rsses_subscription_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_rsses
    ADD CONSTRAINT subscription_rsses_subscription_id_unique UNIQUE (subscription_id);


--
-- Name: subscriptions subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);


--
-- Name: test_singletons test_singletons_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.test_singletons
    ADD CONSTRAINT test_singletons_pkey PRIMARY KEY (key);


--
-- Name: typed_blog_urls typed_blog_urls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.typed_blog_urls
    ADD CONSTRAINT typed_blog_urls_pkey PRIMARY KEY (id);


--
-- Name: user_rsses user_rsses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_rsses
    ADD CONSTRAINT user_rsses_pkey PRIMARY KEY (user_id);


--
-- Name: user_settings user_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_settings
    ADD CONSTRAINT user_settings_pkey PRIMARY KEY (user_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: delayed_jobs_priority; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX delayed_jobs_priority ON public.delayed_jobs USING btree (priority, run_at);


--
-- Name: index_blog_post_categories_on_blog_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_blog_post_categories_on_blog_id ON public.blog_post_categories USING btree (blog_id);


--
-- Name: index_blog_post_category_assignments_on_blog_post_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_blog_post_category_assignments_on_blog_post_id ON public.blog_post_category_assignments USING btree (blog_post_id);


--
-- Name: index_blog_post_category_assignments_on_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_blog_post_category_assignments_on_category_id ON public.blog_post_category_assignments USING btree (category_id);


--
-- Name: index_blog_posts_on_blog_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_blog_posts_on_blog_id ON public.blog_posts USING btree (blog_id);


--
-- Name: index_blogs_on_feed_url_and_version; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_blogs_on_feed_url_and_version ON public.blogs USING btree (feed_url, version);


--
-- Name: index_start_feeds_on_start_page_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_start_feeds_on_start_page_id ON public.start_feeds USING btree (start_page_id);


--
-- Name: index_subscription_posts_on_blog_post_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_subscription_posts_on_blog_post_id ON public.subscription_posts USING btree (blog_post_id);


--
-- Name: index_subscription_posts_on_random_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_subscription_posts_on_random_id ON public.subscription_posts USING btree (random_id);


--
-- Name: index_subscriptions_on_blog_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX index_subscriptions_on_blog_id ON public.subscriptions USING btree (blog_id);


--
-- Name: index_users_on_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_users_on_id ON public.users USING btree (id);


--
-- Name: index_users_on_product_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX index_users_on_product_user_id ON public.users USING btree (product_user_id);


--
-- Name: users_email_without_discarded; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX users_email_without_discarded ON public.users USING btree (email) WHERE (discarded_at IS NULL);


--
-- Name: admin_telemetries bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.admin_telemetries FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: ar_internal_metadata bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.ar_internal_metadata FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_canonical_equality_configs bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_canonical_equality_configs FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_crawl_client_tokens bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_crawl_client_tokens FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_crawl_progresses bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_crawl_progresses FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_crawl_votes bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_crawl_votes FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_discarded_feed_entries bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_discarded_feed_entries FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_missing_from_feed_entries bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_missing_from_feed_entries FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_post_categories bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_post_categories FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_post_category_assignments bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_post_category_assignments FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_post_locks bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_post_locks FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blog_posts bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blog_posts FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: blogs bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.blogs FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: custom_blog_requests bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.custom_blog_requests FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: delayed_jobs bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.delayed_jobs FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: ignored_suggestion_feeds bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.ignored_suggestion_feeds FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: patron_credits bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.patron_credits FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: patron_invoices bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.patron_invoices FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: postmark_bounced_users bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.postmark_bounced_users FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: postmark_bounces bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.postmark_bounces FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: postmark_messages bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.postmark_messages FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: pricing_offers bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.pricing_offers FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: pricing_plans bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.pricing_plans FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: product_events bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.product_events FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: schedules bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.schedules FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: schema_migrations bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.schema_migrations FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: start_feeds bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.start_feeds FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: start_pages bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.start_pages FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: stripe_hanging_sessions bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.stripe_hanging_sessions FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: stripe_subscription_tokens bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.stripe_subscription_tokens FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: stripe_webhook_events bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.stripe_webhook_events FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: subscription_posts bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.subscription_posts FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: subscription_rsses bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.subscription_rsses FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: subscriptions bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.subscriptions FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: test_singletons bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.test_singletons FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: typed_blog_urls bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.typed_blog_urls FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: user_rsses bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.user_rsses FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: user_settings bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.user_settings FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: users bump_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER bump_updated_at BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.bump_updated_at_utc();


--
-- Name: custom_blog_requests custom_blog_requests_blog_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_blog_requests
    ADD CONSTRAINT custom_blog_requests_blog_id_fkey FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE SET NULL;


--
-- Name: custom_blog_requests custom_blog_requests_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_blog_requests
    ADD CONSTRAINT custom_blog_requests_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id);


--
-- Name: custom_blog_requests custom_blog_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_blog_requests
    ADD CONSTRAINT custom_blog_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: blogs fk_blogs_start_feeds; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blogs
    ADD CONSTRAINT fk_blogs_start_feeds FOREIGN KEY (start_feed_id) REFERENCES public.start_feeds(id);


--
-- Name: user_rsses fk_rails_17396fc3a7; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_rsses
    ADD CONSTRAINT fk_rails_17396fc3a7 FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: subscriptions fk_rails_416412a06b; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT fk_rails_416412a06b FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: subscription_rsses fk_rails_647dccf03a; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_rsses
    ADD CONSTRAINT fk_rails_647dccf03a FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id) ON DELETE CASCADE;


--
-- Name: blog_crawl_votes fk_rails_6d5d61b810; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_crawl_votes
    ADD CONSTRAINT fk_rails_6d5d61b810 FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: blog_discarded_feed_entries fk_rails_76729800cc; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_discarded_feed_entries
    ADD CONSTRAINT fk_rails_76729800cc FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: blog_post_category_assignments fk_rails_77d2d90745; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_post_category_assignments
    ADD CONSTRAINT fk_rails_77d2d90745 FOREIGN KEY (blog_post_id) REFERENCES public.blog_posts(id) ON DELETE CASCADE;


--
-- Name: postmark_messages fk_rails_897226ae9c; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.postmark_messages
    ADD CONSTRAINT fk_rails_897226ae9c FOREIGN KEY (subscription_post_id) REFERENCES public.subscription_posts(id) ON DELETE CASCADE;


--
-- Name: blog_canonical_equality_configs fk_rails_9b3fd3910a; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_canonical_equality_configs
    ADD CONSTRAINT fk_rails_9b3fd3910a FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: blog_posts fk_rails_9d677c923b; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_posts
    ADD CONSTRAINT fk_rails_9d677c923b FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: blog_post_categories fk_rails_a7dfb6db3e; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_post_categories
    ADD CONSTRAINT fk_rails_a7dfb6db3e FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: blog_missing_from_feed_entries fk_rails_aecbb1e2bd; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_missing_from_feed_entries
    ADD CONSTRAINT fk_rails_aecbb1e2bd FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: schedules fk_rails_b2b9b40998; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedules
    ADD CONSTRAINT fk_rails_b2b9b40998 FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id) ON DELETE CASCADE;


--
-- Name: subscription_posts fk_rails_b5e611fa3d; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_posts
    ADD CONSTRAINT fk_rails_b5e611fa3d FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id) ON DELETE CASCADE;


--
-- Name: blog_crawl_client_tokens fk_rails_blogs; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_crawl_client_tokens
    ADD CONSTRAINT fk_rails_blogs FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: blog_crawl_progresses fk_rails_blogs; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_crawl_progresses
    ADD CONSTRAINT fk_rails_blogs FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: blog_post_locks fk_rails_c1a166e65f; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_post_locks
    ADD CONSTRAINT fk_rails_c1a166e65f FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: subscriptions fk_rails_c6353e971b; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT fk_rails_c6353e971b FOREIGN KEY (blog_id) REFERENCES public.blogs(id) ON DELETE CASCADE;


--
-- Name: blog_post_category_assignments fk_rails_c6de9c562d; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_post_category_assignments
    ADD CONSTRAINT fk_rails_c6de9c562d FOREIGN KEY (category_id) REFERENCES public.blog_post_categories(id) ON DELETE CASCADE;


--
-- Name: postmark_bounced_users fk_rails_cf5feb15c9; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.postmark_bounced_users
    ADD CONSTRAINT fk_rails_cf5feb15c9 FOREIGN KEY (example_bounce_id) REFERENCES public.postmark_bounces(id) ON DELETE CASCADE;


--
-- Name: user_settings fk_rails_d1371c6356; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_settings
    ADD CONSTRAINT fk_rails_d1371c6356 FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: subscription_posts fk_rails_d857bf4496; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_posts
    ADD CONSTRAINT fk_rails_d857bf4496 FOREIGN KEY (blog_post_id) REFERENCES public.blog_posts(id) ON DELETE CASCADE;


--
-- Name: start_feeds fk_rails_d91add3512; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.start_feeds
    ADD CONSTRAINT fk_rails_d91add3512 FOREIGN KEY (start_page_id) REFERENCES public.start_pages(id) ON DELETE CASCADE;


--
-- Name: postmark_messages fk_rails_e906f9105c; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.postmark_messages
    ADD CONSTRAINT fk_rails_e906f9105c FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id) ON DELETE CASCADE;


--
-- Name: blog_crawl_votes fk_rails_f74b6b39ca; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blog_crawl_votes
    ADD CONSTRAINT fk_rails_f74b6b39ca FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: patron_credits patron_credits_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.patron_credits
    ADD CONSTRAINT patron_credits_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: pricing_offers pricing_offers_plan_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pricing_offers
    ADD CONSTRAINT pricing_offers_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES public.pricing_plans(id);


--
-- Name: pricing_plans pricing_plans_default_offer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pricing_plans
    ADD CONSTRAINT pricing_plans_default_offer_id_fkey FOREIGN KEY (default_offer_id) REFERENCES public.pricing_offers(id);


--
-- Name: stripe_subscription_tokens stripe_subscription_tokens_offer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stripe_subscription_tokens
    ADD CONSTRAINT stripe_subscription_tokens_offer_id_fkey FOREIGN KEY (offer_id) REFERENCES public.pricing_offers(id);


--
-- Name: users users_offer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_offer_id_fkey FOREIGN KEY (offer_id) REFERENCES public.pricing_offers(id);


--
-- PostgreSQL database dump complete
--


INSERT INTO "schema_migrations" (version) VALUES
('20201211221209'),
('20201211235524'),
('20201211235717'),
('20201221203545'),
('20201221203948'),
('20201222003840'),
('20201223002447'),
('20201223003011'),
('20201223003111'),
('20201223003402'),
('20201223003806'),
('20201223005223'),
('20201223190124'),
('20201223192043'),
('20201225012610'),
('20201225213107'),
('20201225224727'),
('20201226000744'),
('20201226002210'),
('20201226011605'),
('20201226012721'),
('20201226012915'),
('20210924002950'),
('20210924005119'),
('20210924021917'),
('20210924030232'),
('20211013194614'),
('20211013195012'),
('20211013195223'),
('20211013215140'),
('20211013220027'),
('20211013220113'),
('20211013224946'),
('20211013225920'),
('20211014202926'),
('20211014231606'),
('20211014232158'),
('20211014232819'),
('20211015001452'),
('20211018223825'),
('20211018224514'),
('20211018233856'),
('20211018234402'),
('20211018234839'),
('20211019223144'),
('20211022194206'),
('20211022194540'),
('20211025203926'),
('20211025210217'),
('20211207204405'),
('20211207205421'),
('20211208011458'),
('20211209232909'),
('20211222010540'),
('20211230004345'),
('20211230014420'),
('20220103221937'),
('20220105004242'),
('20220105210334'),
('20220105233450'),
('20220114233334'),
('20220115001649'),
('20220308005116'),
('20220321220933'),
('20220329191713'),
('20220331171951'),
('20220415183313'),
('20220415201523'),
('20220425183655'),
('20220428234040'),
('20220503200250'),
('20220503204702'),
('20220511235054'),
('20220512003521'),
('20220513002103'),
('20220513002912'),
('20220513010057'),
('20220513015140'),
('20220518233013'),
('20220519004059'),
('20220519014120'),
('20220521001101'),
('20220521002554'),
('20220602184712'),
('20220608213556'),
('20220608214452'),
('20220608215044'),
('20220608215349'),
('20220608215533'),
('20220613192525'),
('20220623000345'),
('20220623230024'),
('20220623233843'),
('20220624232743'),
('20220625000522'),
('20220625002918'),
('20220630213859'),
('20220630232537'),
('20220630232952'),
('20220701002754'),
('20220704213114'),
('20220704230329'),
('20220704230649'),
('20220704233753'),
('20220707003953'),
('20220711225434'),
('20220713013546'),
('20220718195318'),
('20220718195729'),
('20220718200220'),
('20220719212625'),
('20220719212839'),
('20220719213210'),
('20220719214503'),
('20220719215543'),
('20220721221420'),
('20220726015417'),
('20220726025440'),
('20220726221637'),
('20220805192320'),
('20220805234147'),
('20220815191840'),
('20220815192810'),
('20220919185403'),
('20221109212305'),
('20221202021533'),
('20221208232337'),
('20230105004303'),
('20230114001637'),
('20230116214016'),
('20230116221307'),
('20230116230348'),
('20230117220810'),
('20230117232527'),
('20230118004928'),
('20230118010226'),
('20230119010446'),
('20230205064728'),
('20230205065219'),
('20230308011655'),
('20230614044411'),
('20230614051555'),
('20230623031513'),
('20230704085733'),
('20230712025233'),
('20230712025648'),
('20230817035655'),
('20230920040801'),
('20231006024221'),
('20240105034032'),
('20240108100234'),
('20240110082145'),
('20240110082440'),
('20240226094124'),
('20240315060348'),
('20240315105550'),
('20240327044621'),
('20240415075004'),
('20240415084139'),
('20240415093553'),
('20240415100337'),
('20240416094504'),
('20240422084958'),
('20240422100424'),
('20240422111631'),
('20240422112841'),
('20240422113836'),
('20240423112537'),
('20240423214821'),
('20240425211542'),
('20240425212306'),
('20240425220339'),
('20240429215829'),
('20240430220101'),
('20240501133956'),
('20240501214712'),
('20240502165329'),
('20240508213850'),
('20240508225255'),
('20240509165223'),
('20240509182156'),
('20240516182809'),
('20240517110129'),
('20240517111437'),
('20240517194027'),
('20240520160130'),
('20240520192018'),
('20240528135354'),
('20240607185817'),
('20240613152252'),
('20240613175615'),
('20240613193007');

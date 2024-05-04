package migrations

type AddDbTimestamps struct{}

func init() {
	registerMigration(&AddDbTimestamps{})
}

func (m *AddDbTimestamps) Version() string {
	return "20240423112537"
}

func (m *AddDbTimestamps) Up(tx *Tx) {
	tx.MustExec(`alter table schema_migrations add column created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL`)
	tx.MustExec(`alter table schema_migrations add column updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL`)
	tx.MustExec(`create trigger bump_updated_at before update on schema_migrations FOR EACH ROW EXECUTE FUNCTION bump_updated_at_utc();`)

	tx.MustExec(`alter table delayed_jobs alter column created_at set not null`)
	tx.MustExec(`alter table delayed_jobs alter column updated_at set not null`)

	tx.MustExec(`create trigger bump_updated_at before update on stripe_subscription_tokens FOR EACH ROW EXECUTE FUNCTION bump_updated_at_utc();`)

	tx.MustExec(`alter table pricing_offers add column created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL`)
	tx.MustExec(`alter table pricing_offers add column updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL`)
	tx.MustExec(`create trigger bump_updated_at before update on pricing_offers FOR EACH ROW EXECUTE FUNCTION bump_updated_at_utc();`)

	tx.MustExec(`alter table pricing_tiers add column created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL`)
	tx.MustExec(`alter table pricing_tiers add column updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL`)
	tx.MustExec(`create trigger bump_updated_at before update on pricing_tiers FOR EACH ROW EXECUTE FUNCTION bump_updated_at_utc();`)
}

func (m *AddDbTimestamps) Down(tx *Tx) {
	tx.MustExec(`alter table schema_migrations drop column created_at`)
	tx.MustExec(`alter table schema_migrations drop column updated_at`)
	tx.MustExec(`drop trigger bump_updated_at on schema_migrations`)

	tx.MustExec(`alter table delayed_jobs alter column created_at drop not null`)
	tx.MustExec(`alter table delayed_jobs alter column updated_at drop not null`)

	tx.MustExec(`drop trigger bump_updated_at on stripe_subscription_tokens`)

	tx.MustExec(`alter table pricing_offers drop column created_at`)
	tx.MustExec(`alter table pricing_offers drop column updated_at`)
	tx.MustExec(`drop trigger bump_updated_at on pricing_offers`)

	tx.MustExec(`alter table pricing_tiers drop column created_at`)
	tx.MustExec(`alter table pricing_tiers drop column updated_at`)
	tx.MustExec(`drop trigger bump_updated_at on pricing_tiers`)
}

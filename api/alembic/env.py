from alembic import context
from sqlalchemy import create_engine, pool

from app.config import settings

config = context.config
config.set_main_option("sqlalchemy.url", settings.sqlalchemy_url())


def run_migrations_offline() -> None:
    context.configure(url=settings.sqlalchemy_url(), literal_binds=True, dialect_opts={"paramstyle": "named"})
    with context.begin_transaction():
        context.run_migrations()


def run_migrations_online() -> None:
    connectable = create_engine(settings.sqlalchemy_url(), poolclass=pool.NullPool)
    with connectable.connect() as connection:
        context.configure(connection=connection)
        with context.begin_transaction():
            context.run_migrations()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()

import os
from pathlib import Path

import pytest
from alembic import command
from alembic.config import Config
from sqlalchemy import create_engine, text
from sqlalchemy.orm import Session

from app.config import Settings


def _url() -> str:
    raw = os.environ.get(
        "DATABASE_URL",
        "postgres://commerce:commerce@127.0.0.1:5432/commerce_test?sslmode=disable",
    )
    return Settings(database_url=raw).sqlalchemy_url()


@pytest.fixture(scope="session")
def engine():
    url = _url()
    engine = create_engine(url, pool_size=30, max_overflow=10, pool_pre_ping=True)
    with engine.begin() as conn:
        conn.execute(text("DROP SCHEMA public CASCADE"))
        conn.execute(text("CREATE SCHEMA public"))
    ini = Path(__file__).resolve().parents[1] / "alembic.ini"
    cfg = Config(str(ini))
    cfg.set_main_option("sqlalchemy.url", url)
    command.upgrade(cfg, "head")
    yield engine
    engine.dispose()


@pytest.fixture
def session(engine):
    session = Session(engine)
    try:
        yield session
        session.commit()
    except Exception:
        session.rollback()
        raise
    finally:
        session.close()

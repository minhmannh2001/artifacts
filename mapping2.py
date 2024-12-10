import logging
import json
import redis
from sqlalchemy import create_engine, MetaData, Table, Column, Integer, String, Boolean, BigInteger, Text
from sqlalchemy.orm import sessionmaker, scoped_session
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.pool import QueuePool
from sqlalchemy.exc import SQLAlchemyError, OperationalError
from typing import Dict, List, Optional
from urllib.parse import urlparse
import time

# Base for declarative models
Base = declarative_base()

class Mapping(Base):
    __tablename__ = "aoapi_mapping"

    _id = Column(Integer, primary_key=True, autoincrement=True)
    enabled = Column(Boolean)
    creator = Column(String(50))
    last_update = Column(BigInteger)
    mapping_type = Column("type", String(50))
    version = Column(String(50))
    group = Column(String(50))
    description = Column(Text)
    condition = Column(JSONB)
    action = Column(JSONB)
    creation_date = Column(BigInteger)
    tenant = Column(String())

class MappingRetriever:
    def __init__(self,
                 postgres_config: Dict[str, str],
                 redis_config: Dict[str, str],
                 logger: Optional[logging.Logger] = None,
                 pool_size: int = 5,
                 max_overflow: int = 10,
                 pool_timeout: int = 30,
                 pool_recycle: int = 3600
                 ):
        """
        Initialize mapping retrieval module with enhanced connection pool management

        :param postgres_config: Dictionary with PostgreSQL connection parameters
        :param redis_config: Dictionary with Redis connection parameters
        :param logger: Optional logger instance
        :param pool_size: Number of persistent connections to maintain
        :param max_overflow: Maximum number of connections to create beyond pool_size
        :param pool_timeout: Seconds to wait before raising an error when no connection is available
        :param pool_recycle: Time (in seconds) after which a connection is automatically recycled
        """
        # PostgreSQL Connection with Enhanced Pool Management
        try:
            self.pg_engine = create_engine(
                postgres_config["uri"],
                poolclass=QueuePool,
                pool_size=pool_size,
                max_overflow=max_overflow,
                pool_timeout=pool_timeout,
                pool_recycle=pool_recycle,
                pool_pre_ping=True  # Test connection health before using
            )
            
            # Create a scoped session factory for thread-local session management
            self.Session = scoped_session(sessionmaker(bind=self.pg_engine))
        except Exception as e:
            self.logger.error(f"Failed to create PostgreSQL engine: {e}")
            raise

        # Redis Connection
        try:
            self.redis_client = redis.Redis(
                host=redis_config.get('host', 'localhost'),
                port=redis_config.get('port', 6379),
                db=redis_config.get('db', 0),
                password=redis_config.get("password"),
                socket_timeout=5,  # 5 second timeout
                socket_connect_timeout=5,  # 5 second connection timeout
                retry_on_timeout=True,
                max_connections=20  # Prevent connection exhaustion
            )
            
            # Perform a quick connection test
            self.redis_client.ping()
        except redis.RedisError as e:
            self.logger.error(f"Redis connection failed: {e}")
            raise

        # Logger
        self.logger = logger or logging.getLogger(__name__)

    def _get_postgres_session(self):
        """
        Create a new SQLAlchemy session with retry and timeout handling

        :return: SQLAlchemy session object
        """
        max_retries = 3
        retry_delay = 1  # Initial delay between retries

        for attempt in range(max_retries):
            try:
                session = self.Session()
                return session
            except SQLAlchemyError as e:
                self.logger.warning(f"Session creation failed (attempt {attempt + 1}): {e}")
                if attempt < max_retries - 1:
                    time.sleep(retry_delay)
                    retry_delay *= 2  # Exponential backoff
                else:
                    self.logger.error("Failed to create database session after multiple attempts")
                    raise

    def fetch_mappings_from_postgres(self, table):
        """
        Fetch mappings from PostgreSQL with enhanced error handling

        :param table: Mapping table name
        :return: List of mapping dictionaries
        """
        session = None
        try:
            session = self._get_postgres_session()
            
            # Query the Mapping table
            mappings = session.query(Mapping).filter_by(enabled=True).all()

            # Convert the result into a list of dictionaries
            mappings_dict = [
                {
                    "_id": mapping._id,
                    "enabled": mapping.enabled,
                    "creator": mapping.creator,
                    "last_update": mapping.last_update,
                    "mapping_type": mapping.mapping_type,
                    "version": mapping.version,
                    "group": mapping.group,
                    "description": mapping.description,
                    "condition": mapping.condition,
                    "action": mapping.action,
                    "creation_date": mapping.creation_date,
                    "tenant": mapping.tenant,
                }
                for mapping in mappings
            ]
            
            self.logger.info(f"Fetched {len(mappings_dict)} enabled mappings")
            return mappings_dict
        
        except SQLAlchemyError as e:
            self.logger.error(f"Database query error: {e}")
            return []
        
        finally:
            # Ensure session is closed, even if an exception occurs
            if session:
                self.Session.remove()

    def get_mappings(self, mapper_id: str, tenant_info: Dict) -> List[Dict]:
        """
        Retrieve mappings with enhanced Redis caching and error handling

        :param mapper_id: Tenant identifier
        :param tenant_info: Meta information related to tenant
        :return: List of mappings
        """
        redis_key = f"mapper_id:{mapper_id}"

        # Force refresh logic
        force_refresh = tenant_info.get('force_mapping_refresh', False)
        
        if not force_refresh:
            try:
                # Attempt to retrieve from Redis with a timeout
                cached_mappings = self.redis_client.get(redis_key)
                if cached_mappings:
                    self.logger.info(f"Retrieved mappings from Redis for {mapper_id}")
                    return json.loads(cached_mappings)
            except redis.RedisError as e:
                self.logger.warning(f"Redis retrieval error: {e}")

        # Fetch from PostgreSQL
        raw_mappings = self.fetch_mappings_from_postgres(
            tenant_info.get("db_mapping_table_name", "ao_mapping_v2")
        )

        # Additional processing can be added here
        
        # Cache in Redis for future use
        try:
            self.cache_mappings_to_redis(mapper_id, raw_mappings)
        except Exception as e:
            self.logger.error(f"Caching to Redis failed: {e}")

        return raw_mappings
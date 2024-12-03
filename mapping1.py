import logging
import time
import json
import redis
import psycopg2
from typing import Dict, List, Optional

class MappingRetriever:
    def __init__(self, 
                 postgres_config: Dict[str, str], 
                 redis_config: Dict[str, str],
                 logger: logging.Logger = None):
        """
        Initialize mapping retrieval system with PostgreSQL and Redis configurations
        
        :param postgres_config: Dictionary with PostgreSQL connection parameters
        :param redis_config: Dictionary with Redis connection parameters
        :param logger: Optional logger instance
        """
        # PostgreSQL Connection
        self.pg_conn_params = {
            'dbname': postgres_config.get('dbname'),
            'user': postgres_config.get('user'),
            'password': postgres_config.get('password'),
            'host': postgres_config.get('host', 'localhost'),
            'port': postgres_config.get('port', 5432)
        }
        
        # Redis Connection
        self.redis_client = redis.Redis(
            host=redis_config.get('host', 'localhost'),
            port=redis_config.get('port', 6379),
            db=redis_config.get('db', 0)
        )
        
        # Logger
        self.logger = logger or logging.getLogger(__name__)
    
    def _get_postgres_connection(self):
        """
        Create a new PostgreSQL connection
        
        :return: PostgreSQL connection object
        """
        return psycopg2.connect(**self.pg_conn_params)
    
    def fetch_mappings_from_postgres(self, tenant_id: str) -> List[Dict]:
        """
        Fetch mappings from PostgreSQL for a specific tenant
        
        :param tenant_id: Tenant identifier
        :return: List of mapping dictionaries
        """
        try:
            with self._get_postgres_connection() as conn:
                with conn.cursor() as cur:
                    cur.execute("""
                        SELECT mapping_data 
                        FROM mappings 
                        WHERE tenant_id = %s AND is_active = true
                    """, (tenant_id,))
                    
                    mappings = [row[0] for row in cur.fetchall()]
                    self.logger.info(f"Fetched {len(mappings)} mappings for tenant {tenant_id}")
                    return mappings
        except Exception as e:
            self.logger.error(f"Error fetching mappings from PostgreSQL: {e}")
            return []
    
    def parse_mappings(self, raw_mappings: List[Dict]) -> List[Dict]:
        """
        Parse and transform raw mappings
        
        :param raw_mappings: Raw mapping dictionaries
        :return: Parsed and transformed mappings
        """
        # Implement your specific parsing logic here
        parsed_mappings = []
        for mapping in raw_mappings:
            try:
                # Add your parsing/transformation logic
                parsed_mapping = {
                    'id': mapping.get('_id') or mapping.get('id'),
                    'tenant': mapping.get('tenant'),
                    'action': mapping.get('action', {}),
                    'condition': mapping.get('condition', {})
                }
                parsed_mappings.append(parsed_mapping)
            except Exception as e:
                self.logger.warning(f"Failed to parse mapping: {e}")
        
        return parsed_mappings
    
    def cache_mappings_to_redis(self, tenant_id: str, mappings: List[Dict], 
                                 expiry_seconds: int = 3600):
        """
        Cache parsed mappings to Redis
        
        :param tenant_id: Tenant identifier
        :param mappings: Parsed mappings
        :param expiry_seconds: Redis key expiration time
        """
        try:
            redis_key = f"mappings:{tenant_id}"
            self.redis_client.setex(
                redis_key, 
                expiry_seconds, 
                json.dumps(mappings)
            )
            self.logger.info(f"Cached {len(mappings)} mappings for tenant {tenant_id}")
        except Exception as e:
            self.logger.error(f"Error caching mappings to Redis: {e}")
    
    def get_mappings(self, tenant_id: str, force_refresh: bool = False) -> List[Dict]:
        """
        Retrieve mappings with Redis caching strategy
        
        :param tenant_id: Tenant identifier
        :param force_refresh: Force fetch from PostgreSQL
        :return: List of mappings
        """
        redis_key = f"mappings:{tenant_id}"
        
        # Check Redis first unless forced refresh
        if not force_refresh:
            try:
                cached_mappings = self.redis_client.get(redis_key)
                if cached_mappings:
                    self.logger.info(f"Retrieved mappings from Redis for tenant {tenant_id}")
                    return json.loads(cached_mappings)
            except Exception as e:
                self.logger.warning(f"Redis retrieval error: {e}")
        
        # Fetch from PostgreSQL
        raw_mappings = self.fetch_mappings_from_postgres(tenant_id)
        parsed_mappings = self.parse_mappings(raw_mappings)
        
        # Cache in Redis for future use
        self.cache_mappings_to_redis(tenant_id, parsed_mappings)
        
        return parsed_mappings

def create_mapping_retriever(tenant_info: Dict) -> MappingRetriever:
    """
    Factory function to create MappingRetriever with tenant configuration
    
    :param tenant_info: Tenant configuration dictionary
    :return: MappingRetriever instance
    """
    postgres_config = {
        'dbname': tenant_info.get('pg_dbname'),
        'user': tenant_info.get('pg_user'),
        'password': tenant_info.get('pg_password'),
        'host': tenant_info.get('pg_host', 'localhost'),
        'port': tenant_info.get('pg_port', 5432)
    }
    
    redis_config = {
        'host': tenant_info.get('redis_host', 'localhost'),
        'port': tenant_info.get('redis_port', 6379),
        'db': tenant_info.get('redis_db', 0)
    }
    
    logger = logging.getLogger("root")
    return MappingRetriever(postgres_config, redis_config, logger)

def modify_get_and_transform_mappings(event_store, _mappings, tenant_info, logger):
    """
    Modified version of _get_and_transform_mappings to use MappingRetriever
    
    :param event_store: Original event store (can be removed if no longer needed)
    :param _mappings: Previous mappings (for compatibility)
    :param tenant_info: Tenant configuration
    :param logger: Logger instance
    :return: Updated mappings
    """
    mapping_retriever = create_mapping_retriever(tenant_info)
    
    now = int(time.time() * 1000)
    if now > tenant_info.get("update_time", 0):
        # Periodic refresh logic
        tenant_info["update_time"] = now + tenant_info.get("update_interval", 5) * 1000
        
        raw_mappings = mapping_retriever.get_mappings(
            tenant_info.get('tenant_id', 'default'), 
            force_refresh=tenant_info.get('force_mapping_refresh', False)
        )
        
        # Optional: Apply any tenant-specific conditions
        if tenant_info.get("iml_on"):
            # Placeholder for any specific mapping transformations
            pass
        
        return raw_mappings
    
    return _mappings

# Update the main processing function to use this new mapping retrieval
def create_child_main(tenant_info, update_interval, controller):
    def child_main():
        # Existing setup code remains the same
        # Replace _get_and_transform_mappings with modify_get_and_transform_mappings
        # ... rest of the function remains similar
        pass
    
    return child_main
import asyncio
import aiohttp
import logging
import time
from enum import Enum
from typing import Dict, Any, Optional

class Result(Enum):
    SUCCEED = "succeed"
    INVALID_EVENT = "invalid_event"
    ENGINE_INTERNAL_ERROR = "engine_internal_error"
    ENGINE_CONNECTION_ERROR = "engine_connection_error"
    TIMEOUT = "timeout"
    UNDEFINED_ERROR = "undefined_error"

class AsyncOrenctlEngine:
    def __init__(self, ingest_api: str, access_token: Optional[str] = None, api_key: Optional[str] = None):
        """
        Initialize the async engine client with authentication headers
        
        :param ingest_api: Base URL for ingestion API
        :param access_token: Bearer token for authentication
        :param api_key: API key for authentication
        """
        self.ingest_api = ingest_api
        self.headers = {}
        
        if access_token:
            self.headers['Authorization'] = f"Bearer {access_token}"
        if api_key:
            self.headers['X-API-KEY'] = api_key

    async def ingest(
        self, 
        payload: Dict[str, Any], 
        tenant_info: Dict[str, Any], 
        tenant: Optional[str] = None
    ) -> Result:
        """
        Async method to ingest payload to the orchestration engine
        
        :param payload: Payload to be ingested
        :param tenant_info: Tenant-specific information
        :param tenant: Specific tenant identifier
        :return: Result of the ingestion
        """
        logger = logging.getLogger(__name__)
        
        # Update headers with tenant 
        headers = self.headers.copy()
        if tenant:
            headers['Tenant'] = tenant

        try:
            async with aiohttp.ClientSession() as session:
                async with session.post(
                    self.ingest_api, 
                    json=payload, 
                    headers=headers,
                    timeout=tenant_info.get('request_timeout', 30)
                ) as res:
                    logger.info(
                        f"[Tenant: {tenant or 'Tenant in API-KEY'}] "
                        f"Ingested payload: {payload}. "
                        f"Response: {res.status} - {await res.text()}"
                    )
                    
                    if res.status == 200:
                        return Result.SUCCEED
                    elif res.status == 500:
                        return Result.ENGINE_INTERNAL_ERROR
        
        except aiohttp.ClientConnectionError:
            return Result.ENGINE_CONNECTION_ERROR
        except asyncio.TimeoutError:
            return Result.TIMEOUT
        except Exception as e:
            logger.error(f"Unexpected error during ingestion: {e}")
            return Result.UNDEFINED_ERROR

async def run_playbook(
    event: Dict[str, Any],
    tenant_info: Dict[str, Any],
    playbook_id: Optional[str] = None,
    config_id: Optional[str] = None,
    extra_info: Optional[Dict[str, Any]] = None,
    **kwargs
) -> Result:
    """
    Async method to run a playbook via AOEngine APIs
    
    :param event: Event data
    :param tenant_info: Tenant-specific information
    :param playbook_id: ID of the playbook to run
    :param config_id: Configuration ID
    :param extra_info: Additional information
    :return: Result of playbook execution
    """
    logger = logging.getLogger(__name__)

    tenant_id = event.get("tenant", tenant_info["tenant_id"])
    engine_api_uri = f'{tenant_info["engine_api_uri"]}/{tenant_id}/engine'
    uri = f"{engine_api_uri}/_run_playbook"

    # Extract playbook_id and config_id from event data if not provided
    if event.get("data", None):
        playbook_id = playbook_id or event["data"].get("playbook_id")
        config_id = config_id or event["data"].get("config_id")

    if not playbook_id:
        logger.error("Playbook id is not specified. Aborted.")
        return Result.INVALID_EVENT

    # Attach additional information
    event["extra_info"] = extra_info

    body = {"playbook_id": playbook_id, "input_event": event}
    if config_id:
        body["config_id"] = config_id

    try:
        async with aiohttp.ClientSession() as session:
            async with session.post(
                uri, 
                json=body, 
                timeout=tenant_info.get('request_timeout', 30)
            ) as res:
                logger.info({
                    "title": "Calling aoengine.",
                    "request": {
                        "url": uri,
                        "method": "POST",
                        "body": str(body)[:100],
                    },
                    "response": {
                        "status_code": res.status,
                        "body": (await res.text())[:100] + "...",
                    },
                })
                
                if res.status == 200:
                    return Result.SUCCEED
                elif res.status == 500:
                    return Result.ENGINE_INTERNAL_ERROR
    
    except aiohttp.ClientConnectionError:
        return Result.ENGINE_CONNECTION_ERROR
    except asyncio.TimeoutError:
        return Result.TIMEOUT
    except Exception as e:
        logger.error(f"Unexpected error running playbook: {e}")
        return Result.UNDEFINED_ERROR

async def process_mappings_async(
    matched_mappings: list, 
    event: Dict[str, Any], 
    tenant_info: Dict[str, Any], 
    target_data: Optional[Dict[str, Any]] = None,
    kafka_client: Any = None,
    partition: Optional[int] = None,
    offset: Optional[int] = None,
    time_commit: Optional[float] = None
) -> None:
    """
    Async method to process mappings with parallel execution
    
    :param matched_mappings: List of mappings to process
    :param event: Event data
    :param tenant_info: Tenant-specific information
    :param target_data: Additional target data
    :param kafka_client: Kafka client for committing offsets
    :param partition: Kafka partition
    :param offset: Kafka offset
    :param time_commit: Commit time
    """
    logger = logging.getLogger(__name__)
    
    # Result codes that require retry
    result_for_retry = {
        Result.ENGINE_INTERNAL_ERROR, 
        Result.ENGINE_CONNECTION_ERROR, 
        Result.TIMEOUT
    }
    
    engine = AsyncOrenctlEngine(
        ingest_api=tenant_info.get('ingest_api'), 
        access_token=tenant_info.get('access_token'),
        api_key=tenant_info.get('api_key')
    )

    for mapping in matched_mappings:
        next_condition = False
        retry_count = 0
        max_retries = 3

        while not next_condition and retry_count < max_retries:
            try:
                if mapping.get('action', {}).get('playbook_type') == "aoengine":
                    playbook_id, config_id = prepare_aoengine_payload(event, mapping)

                    kw = {
                        "tenant_info": tenant_info,
                        "playbook_id": playbook_id,
                        "extra_info": {
                            "mapping_id": mapping.get('id')
                        }
                    }

                    if config_id:
                        kw.update({"config_id": config_id})

                    if target_data is not None:
                        event['data'].update(target_data)
                    
                    res = await run_playbook(event=event, **kw)
                else:
                    payload = prepare_orenctl_payload(
                        event, target_data, mapping, tenant_info
                    )
                    res = await engine.ingest(payload, tenant_info, mapping.get("tenant"))

                if res in result_for_retry:
                    logger.error(f"Engine error: {res.value}")
                    logger.error(
                        f"Commit current offset to kafka:"
                        f" Partition {partition} - Offset {offset}."
                    )
                    if kafka_client:
                        kafka_client._commit({partition: offset})
                    
                    retry_count += 1
                    await asyncio.sleep(1)  # Async sleep instead of time.sleep
                else:
                    # Commit logic remains similar
                    timeleft = time_commit - time.time() * 1000 if time_commit else 0
                    if timeleft < 1.5:
                        logger.info("Time to commit less than 1.5 second.")
                        logger.info(
                            f"Commit next offset to kafka:"
                            f" Partition {partition} - Offset {offset + 1}."
                        )

                        try:
                            kafka_client._commit({partition: offset + 1})
                        except Exception as ex:
                            logger.error(f"Error happened: {ex}", exc_info=True)
                            await asyncio.sleep(10)
                    
                    next_condition = True

            except Exception as ex:
                logger.error(f"Unexpected error in mapping processing: {ex}", exc_info=True)
                retry_count += 1
                await asyncio.sleep(1)

        logger.info("Next event.")

# Note: You'll need to implement these functions based on your existing logic
def prepare_aoengine_payload(event, mapping):
    # Implement payload preparation logic
    pass

def prepare_orenctl_payload(event, target_data, mapping, tenant_info):
    # Implement payload preparation logic
    pass
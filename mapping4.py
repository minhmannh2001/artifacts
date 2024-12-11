import concurrent.futures
import multiprocessing
import requests
import time
import logging
from typing import List, Dict, Any, Optional

def optimize_mapping_processing(
    matched_mappings: List[Dict[str, Any]], 
    event: Dict[str, Any], 
    tenant_info: Dict[str, Any]
) -> List[Result]:
    """
    Performance optimization techniques for processing mappings
    """
    # Method 1: Concurrent Execution using ThreadPoolExecutor
    def process_mapping_with_threads(max_workers=None):
        """
        Process mappings concurrently using threads
        
        :param max_workers: Maximum number of concurrent threads
        :return: List of processing results
        """
        results = []
        with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
            # Create a list of future objects
            future_to_mapping = {
                executor.submit(process_single_mapping, mapping, event, tenant_info): mapping 
                for mapping in matched_mappings
            }
            
            # Collect results as they complete
            for future in concurrent.futures.as_completed(future_to_mapping):
                mapping = future_to_mapping[future]
                try:
                    result = future.result()
                    results.append(result)
                except Exception as exc:
                    logging.error(f"Mapping {mapping.get('id')} generated an exception: {exc}")
        
        return results

    # Method 2: Multiprocessing for CPU-bound tasks
    def process_mapping_with_multiprocessing(max_processes=None):
        """
        Process mappings using multiple processes
        
        :param max_processes: Maximum number of concurrent processes
        :return: List of processing results
        """
        # Use the number of CPU cores if not specified
        if max_processes is None:
            max_processes = multiprocessing.cpu_count()
        
        with multiprocessing.Pool(processes=max_processes) as pool:
            # Use starmap to pass multiple arguments
            results = pool.starmap(
                process_single_mapping, 
                [(mapping, event, tenant_info) for mapping in matched_mappings]
            )
        
        return results

    # Method 3: Batched Processing
    def process_mapping_in_batches(batch_size=5):
        """
        Process mappings in smaller batches to control resource usage
        
        :param batch_size: Number of mappings to process in each batch
        :return: List of processing results
        """
        results = []
        for i in range(0, len(matched_mappings), batch_size):
            batch = matched_mappings[i:i+batch_size]
            
            # Process batch with ThreadPoolExecutor
            with concurrent.futures.ThreadPoolExecutor() as executor:
                batch_futures = [
                    executor.submit(process_single_mapping, mapping, event, tenant_info) 
                    for mapping in batch
                ]
                
                # Collect batch results
                for future in concurrent.futures.as_completed(batch_futures):
                    try:
                        result = future.result()
                        results.append(result)
                    except Exception as exc:
                        logging.error(f"Batch mapping processing generated an exception: {exc}")
        
        return results

    # Method 4: Rate-Limited Processing
    def process_mapping_with_rate_limit(
        max_concurrent=5, 
        min_interval=0.2
    ):
        """
        Process mappings with controlled concurrency and rate limiting
        
        :param max_concurrent: Maximum number of concurrent requests
        :param min_interval: Minimum time between request batches
        :return: List of processing results
        """
        results = []
        for i in range(0, len(matched_mappings), max_concurrent):
            batch = matched_mappings[i:i+max_concurrent]
            
            with concurrent.futures.ThreadPoolExecutor(max_workers=max_concurrent) as executor:
                # Submit batch of mappings
                future_to_mapping = {
                    executor.submit(process_single_mapping, mapping, event, tenant_info): mapping 
                    for mapping in batch
                }
                
                # Collect results
                for future in concurrent.futures.as_completed(future_to_mapping):
                    mapping = future_to_mapping[future]
                    try:
                        result = future.result()
                        results.append(result)
                    except Exception as exc:
                        logging.error(f"Mapping {mapping.get('id')} generated an exception: {exc}")
            
            # Rate limiting between batches
            time.sleep(min_interval)
        
        return results

def process_single_mapping(
    mapping: Dict[str, Any], 
    event: Dict[str, Any], 
    tenant_info: Dict[str, Any]
) -> Result:
    """
    Process a single mapping with retry mechanism
    
    :param mapping: Mapping configuration
    :param event: Event data
    :param tenant_info: Tenant information
    :return: Processing result
    """
    logger = logging.getLogger(__name__)
    max_retries = 3
    
    for attempt in range(max_retries):
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
                
                # Optional: Update event data if needed
                # if target_data is not None:
                #     event['data'].update(target_data)
                
                res = run_playbook(event=event, **kw)
            else:
                payload = prepare_orenctl_payload(
                    event, target_data, mapping, tenant_info
                )
                res = engine.ingest(payload, tenant_info, mapping.get("tenant"))
            
            # If successful, return result
            if res not in {Result.ENGINE_INTERNAL_ERROR, Result.ENGINE_CONNECTION_ERROR, Result.TIMEOUT}:
                return res
            
            # Log retry attempt
            logger.warning(f"Attempt {attempt + 1} failed with result: {res}")
            
            # Exponential backoff
            time.sleep(2 ** attempt)
        
        except Exception as e:
            logger.error(f"Error processing mapping: {e}")
    
    # If all retries fail
    return Result.UNDEFINED_ERROR

# Performance Optimization Recommendations
def performance_recommendations():
    """
    Key recommendations for improving request processing performance
    """
    recommendations = [
        "1. Use ThreadPoolExecutor for I/O-bound tasks (network requests)",
        "2. Use multiprocessing for CPU-intensive tasks",
        "3. Implement rate limiting to prevent overwhelming external services",
        "4. Add exponential backoff for retries",
        "5. Consider connection pooling (requests.Session() or aiohttp)",
        "6. Compress request payloads if possible",
        "7. Use connection keep-alive",
        "8. Monitor and log performance metrics"
    ]
    
    return recommendations

# Monitoring and Profiling Helper
def profile_mapping_processing(func):
    """
    Decorator to profile mapping processing performance
    """
    def wrapper(*args, **kwargs):
        start_time = time.time()
        results = func(*args, **kwargs)
        end_time = time.time()
        
        logging.info(f"Processing Time: {end_time - start_time:.2f} seconds")
        logging.info(f"Total Mappings Processed: {len(results)}")
        logging.info(f"Successful Results: {sum(1 for r in results if r == Result.SUCCEED)}")
        
        return results
    return wrapper

# Example usage with profiling
@profile_mapping_processing
def main():
    # Your existing setup code here
    results = process_mapping_with_threads(max_workers=10)
    return results
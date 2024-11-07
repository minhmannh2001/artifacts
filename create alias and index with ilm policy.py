from elasticsearch import Elasticsearch

def configure_elasticsearch(es_host="localhost", es_port=9200):
    """
    Configure Elasticsearch with ILM policy, index template, and initial index.
    
    Args:
        es_host (str): Elasticsearch host
        es_port (int): Elasticsearch port
    """
    # Initialize Elasticsearch client
    es = Elasticsearch([{'host': es_host, 'port': es_port}])
    
    # Create ILM policy
    ilm_policy = {
        "policy": {
            "phases": {
                "hot": {
                    "actions": {
                        "rollover": {
                            "max_age": "7d"
                        }
                    }
                },
                "delete": {
                    "min_age": "0m",
                    "actions": {
                        "delete": {}
                    }
                }
            }
        }
    }
    
    # Create index template
    index_template = {
        "index_patterns": ["vcs_cycir_playbook_debug_log-*"],
        "settings": {
            "index": {
                "lifecycle": {
                    "name": "playbook_debug_log_rollover_policy",
                    "rollover_alias": "vcs_cycir_playbook_debug_log"
                }
            }
        }
    }
    
    # Create initial index with alias
    initial_index = {
        "aliases": {
            "vcs_cycir_playbook_debug_log": {
                "is_write_index": True
            }
        }
    }
    
    try:
        # Put ILM policy
        es.ilm.put_lifecycle(
            name="playbook_debug_log_rollover_policy",
            body=ilm_policy
        )
        print("Successfully created ILM policy")
        
        # Put index template
        es.indices.put_template(
            name="playbook_debug_log_template",
            body=index_template
        )
        print("Successfully created index template")
        
        # Create initial index with alias
        es.indices.create(
            index="vcs_cycir_playbook_debug_log-000001",
            body=initial_index
        )
        print("Successfully created initial index with alias")
        
        return True
        
    except Exception as e:
        print(f"Error occurred: {str(e)}")
        return False

if __name__ == "__main__":
    # Example usage
    configure_elasticsearch()
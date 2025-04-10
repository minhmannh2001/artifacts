@add_user_update
def create_alerts_batch(body=None):
    """
    Create multiple alerts with their entities and events in batch, supporting multi-tenant
    :param body: list of alert data with format: 
                [{"tenant": "tenant_id", 
                  "alert": {...}, 
                  "entity": [...],
                  "events": [...]
                }]
    :return: JSON response with created alerts
    """
    action_logger.info(action=Action.ALERT_CREATE_BATCH, state=Action.STATE_START)
    
    if not isinstance(body, list):
        raise api_error.InvalidParameter(error_code=4001000, params="body")
        
    results = []
    try:
        for item in body:
            tenant = item.get("tenant")
            alert_data = item.get("alert", {})
            entities = item.get("entity", [])
            events = item.get("events", [])

            if not tenant:
                raise api_error.InvalidParameter(error_code=4001000, params="tenant")

            # Modify params for new alert
            alert_data["status"] = Alert.Status.OPEN
            alert_data["unread"] = True
            alert_data["created"] = get_time()
            alert_data["sla_expired"] = False

            # Remove ID if present to avoid conflicts
            if alert_data.get("_id") or alert_data.get("_id") == 0:
                alert_data["_alert_id"] = alert_data["_id"]
                del alert_data["_id"]

            # Validate severity
            SEVERITY_LIST = [
                Alert.Severity.LOW,
                Alert.Severity.MEDIUM,
                Alert.Severity.HIGH,
                Alert.Severity.CRITICAL,
            ]
            if alert_data.get("severity") not in SEVERITY_LIST:
                raise api_error.InvalidParameter(
                    error_code=4001000,
                    params="severity",
                    payload={"error_prams": "severity"},
                )

            # Handle alert size limits
            max_alert_size = current_app.config.get("MAX_ALERT_SIZE", 2)
            trim_fraction = current_app.config.get("TRIM_FRACTION_OF_ALERT_SIZE", 0.1)
            reduce_alert(alert_data, max_size_in_bytes=max_alert_size * 1024 * 1024, trim_fraction=trim_fraction)
            data_create = truncate_string_value(alert_data)
            
            # Create alert
            created_alert = records_store.create(
                tenant=tenant, table=TABLE, data=data_create, commit=False
            )
            alert_id = created_alert.get("alert_id")

            # Handle entities if present
            if entities and len(entities) > 0:
                entity_type_keys = []
                entity_values = []
                get_list_key_and_value_of_entities(entities, entity_type_keys, entity_values)

                # Check existing entities
                sql_backend = records.get_db(tenant)
                session.instance = sql_backend.session
                entities_filter = (session.query(Entity._id, Entity.entity_type_key, Entity.value)
                                   .filter(Entity.entity_type_key.in_(entity_type_keys))
                                   .filter(Entity.value.in_(entity_values)).all())

                entity_maps = {}
                for entity in entities_filter:
                    entity_maps[f'{entity[1]}_{entity[2]}'] = entity[0]

                # Process each entity
                for entity in entities:
                    entity_id = None
                    entity_key_value_map = f'{entity.get("entity_type_key")}_{entity.get("value")}'
                    
                    if entity_maps.get(entity_key_value_map):
                        entity_id = entity_maps.get(entity_key_value_map)
                    else:
                        entity["created"] = get_time()
                        entity["user_update"] = ActorAPI().get_actor()
                        entity["last_updated"] = get_time()
                        entity["tenant"] = tenant
                        if 'alert_id' in entity:
                            del entity['alert_id']
                        created_entity = records_store.create(
                            tenant=tenant, table='entity', data=entity, commit=False
                        )
                        entity_id = created_entity.get("_id")

                    # Create entity-alert mapping
                    _ = records_store.create(
                        tenant=tenant,
                        table="entity_alert_case_mapping",
                        data={
                            "alert_id": alert_id,
                            "entity_id": entity_id,
                            "tenant": tenant
                        },
                        commit=False
                    )

            # Handle events if present
            if events and len(events) > 0:
                for event in events:
                    event_id = str(uuid.uuid4())  # Generate unique ID for event
                    artifact_event = {
                        "_id": event_id,
                        "_alert_id": alert_id,
                        "event_id": event.get("event_id"),
                        "_data": event.get("data", {}),
                        "tenant": tenant
                    }
                    
                    # Create artifact event
                    _ = records_store.create(
                        tenant=tenant,
                        table="alert_artifact_event",
                        data=artifact_event,
                        commit=False
                    )

                    # Update alert with events reference if needed
                    if "events" not in created_alert:
                        created_alert["events"] = []
                    created_alert["events"].append(event_id)

            results.append({
                "tenant": tenant,
                "alert": created_alert
            })

        # Commit all changes at once
        records_store.commit(tenant)
        action_logger.info(
            action=Action.ALERT_CREATE_BATCH, state=Action.STATE_SUCCESS
        )
        return jsonify({"data": results}), 201

    except Exception as e:
        records_store.rollback(tenant)
        action_logger.info(
            action=Action.ALERT_CREATE_BATCH, error=str(e), state=Action.STATE_ERROR
        )
        if isinstance(e, IntegrityError):
            raise api_error.InvalidParameter(
                error_code=4001001, params="alert_id"
            ) from e
        raise api_error.InternalServerError()

def create_alert_with_entity(tenant, body):
    try:
        commit = False
        created_alert, status = create_alert(tenant, body, commit)
        alert_id = created_alert.json["alert_id"]
        
        # Handle events creation
        if "events" in body and body["events"] is not None and len(body["events"]) > 0:
            events = body.get("events")
            request_id = uuid.uuid4()
            try:
                current_app.logger.info(f"{str(request_id)}, events={events}")
                current_app.logger.info(
                    f"{str(request_id)}, tenant={ActorAPI.get_tenant()}, specific_tenant={ActorAPI.get_master_specific_tenant()}"
                )
            except Exception as e:
                current_app.logger.error(e)

            for event in events:
                event["_alert_id"] = alert_id
                try:
                    _ = records_store.create(
                        tenant=tenant, table="alert_artifact_event", data=event, commit=commit
                    )
                    action_logger.info(
                        action=Action.ALERT_ARTIFACT_CREATE, state=Action.STATE_SUCCESS
                    )
                except Exception as e:
                    action_logger.info(
                        action=Action.ALERT_ARTIFACT_CREATE,
                        error=str(e),
                        state=Action.STATE_ERROR,
                    )
                    if isinstance(e, IntegrityError):
                        raise api_error.InvalidParameter(
                            error_code=4001001, params="_id"
                        ) from e
                    raise api_error.InternalServerError()

        # Handle entities creation
        if "entity" in body and body["entity"] is not None and len(body["entity"]) > 0:
            entity_type_keys = []
            entity_values = []
            entities = body.get("entity")
            get_list_key_and_value_of_entities(entities, entity_type_keys, entity_values)
            # check exist entity
            sql_backend = records.get_db(tenant)
            session.instance = sql_backend.session
            entities_filter = (session.query(Entity._id, Entity.entity_type_key, Entity.value)
                               .filter(Entity.entity_type_key.in_(entity_type_keys))
                               .filter(Entity.value.in_(entity_values)).all())

            entity_maps = {}
            for entity in entities_filter:
                entity_maps[f'{entity[1]}_{entity[2]}'] = entity[0]

            for entity in entities:
                entity_id = None
                entity_key_value_map = f'{entity.get("entity_type_key")}_{entity.get("value")}'
                if entity_maps.get(entity_key_value_map):
                    entity_id = entity_maps.get(entity_key_value_map)
                else:
                    entity["created"] = get_time()
                    entity["user_update"] = ActorAPI().get_actor()
                    entity["last_updated"] = get_time()
                    if 'alert_id' in entity:
                        del entity['alert_id']
                    created_entity = records_store.create(
                        tenant=tenant, table='entity', data=entity, commit=commit
                    )
                    entity_id = created_entity.get("_id")

                # save to entity_alert_mapping
                _ = records_store.create(
                    tenant=tenant,
                    table="entity_alert_case_mapping",
                    data={
                        "alert_id": alert_id,
                        "entity_id": entity_id
                    },
                    commit=commit
                )
        
        records_store.commit(tenant)
        return created_alert, 201
    except Exception as e:
        current_app.logger.error(e)
        records_store.rollback(tenant)
        raise e

#!/bin/bash
# Professional Slack Notifications (replaces 8398a7/action-slack@v3)

set -euo pipefail

source /scripts/common/logging.sh

get_slack_webhook_url() {
    # Try multiple sources for webhook URL
    if [[ -n "${SLACK_WEBHOOK_URL:-}" ]]; then
        echo "$SLACK_WEBHOOK_URL"
    elif [[ -f "/tmp/slack_webhook_url" ]]; then
        cat /tmp/slack_webhook_url
    else
        log_warning "No Slack webhook URL found"
        return 1
    fi
}

get_notification_emoji() {
    local status="$1"
    
    case "$status" in
        "started") echo "🚧" ;;
        "success") echo "✅" ;;
        "failure") echo "❗" ;;
        "cancelled") echo "❌" ;;
        *) echo "ℹ️" ;;
    esac
}

get_slack_author() {
    # Load Slack user mapping
    if [[ -f "/tmp/slack_user_id" ]]; then
        cat /tmp/slack_user_id
    else
        echo "$GITHUB_ACTOR"
    fi
}

get_build_url() {
    echo "${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}"
}

get_cc_devs() {
    local status="$1"
    
    # Load CC settings from user mapping
    if [[ "$status" == "started" && -f "/tmp/slack_cc_devs_start" ]]; then
        cat /tmp/slack_cc_devs_start
    elif [[ "$status" == "failure" && -f "/tmp/slack_cc_devs_failure" ]]; then
        cat /tmp/slack_cc_devs_failure
    else
        echo ""
    fi
}

format_slack_payload() {
    local status="$1"
    local emoji=$(get_notification_emoji "$status")
    local slack_author=$(get_slack_author)
    local build_url=$(get_build_url)
    local cc_devs=$(get_cc_devs "$status")
    
    # Load metadata
    local version="${VERSION:-$(cat /tmp/deploy_version 2>/dev/null || echo 'unknown')}"
    local branch="${GITHUB_REF_NAME:-$(cat /tmp/git_branch 2>/dev/null || echo 'unknown')}"
    local message="$(cat /tmp/git_message 2>/dev/null || echo 'Manual deployment')"
    
    # Format duration for success notifications
    local duration_text=""
    if [[ "$status" == "success" && -f "/tmp/deploy_duration" ]]; then
        local duration=$(cat /tmp/deploy_duration)
        duration_text=" (took: $duration)"
    fi
    
    # Build notification text based on status
    local notification_text
    case "$status" in
        "started")
            notification_text="$emoji *<$build_url|STARTED>* deploy *${STACK_NAME}* to *${ENVIRONMENT}* (v$version) by <@$slack_author> $cc_devs"
            ;;
        "success")
            notification_text="$emoji *<$build_url|SUCCESS>* deploy *${STACK_NAME}* to *${ENVIRONMENT}* (v$version) ($branch) - $message by <@$slack_author>$duration_text"
            ;;
        "failure")
            notification_text="$emoji *<$build_url|FAILURE>* deploy *${STACK_NAME}* to *${ENVIRONMENT}* ($branch) - $message by <@$slack_author> $cc_devs"
            ;;
        "cancelled")
            notification_text="$emoji *<$build_url|CANCELLED>* deploy *${STACK_NAME}* to *${ENVIRONMENT}* ($branch) - $message by <@$slack_author> $cc_devs"
            ;;
    esac
    
    # Create Slack block payload
    cat <<EOF
{
    "blocks": [
        {
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": "$notification_text"
            }
        }
    ]
}
EOF
}

send_slack_notification() {
    local status="$1"
    local webhook_url
    
    if ! webhook_url=$(get_slack_webhook_url); then
        log_info "Skipping Slack notification - no webhook URL configured"
        return 0
    fi
    
    log_info "Sending Slack notification for status: $status"
    
    local payload
    payload=$(format_slack_payload "$status")
    
    # Send notification with retry logic
    local max_retries=3
    local retry_count=0
    
    while [ $retry_count -lt $max_retries ]; do
        if curl -f -X POST \
            -H "Content-type: application/json" \
            --data "$payload" \
            --max-time 30 \
            --silent \
            --show-error \
            "$webhook_url"; then
            
            log_info "✅ Slack notification sent successfully"
            return 0
        else
            retry_count=$((retry_count + 1))
            if [ $retry_count -lt $max_retries ]; then
                log_warning "⚠️ Slack notification failed, retrying in 5 seconds (attempt $retry_count/$max_retries)"
                sleep 5
            fi
        fi
    done
    
    log_error "❌ Failed to send Slack notification after $max_retries attempts"
    return 1
}

send_discord_notification() {
    local status="$1"
    
    # Check if Discord webhook is configured
    if [[ -z "${DISCORD_WEBHOOK_URL:-}" ]] && [[ ! -f "/tmp/discord_webhook_url" ]]; then
        log_info "Skipping Discord notification - no webhook URL configured"
        return 0
    fi
    
    local webhook_url="${DISCORD_WEBHOOK_URL:-$(cat /tmp/discord_webhook_url 2>/dev/null || echo '')}"
    if [[ -z "$webhook_url" ]]; then
        return 0
    fi
    
    log_info "Sending Discord notification for status: $status"
    
    # Convert Slack payload to Discord format
    local emoji=$(get_notification_emoji "$status")
    local build_url=$(get_build_url)
    local version="${VERSION:-$(cat /tmp/deploy_version 2>/dev/null || echo 'unknown')}"
    
    local discord_payload
    discord_payload=$(cat <<EOF
{
    "embeds": [
        {
            "title": "Simple Container Deployment",
            "description": "$emoji **${status^^}** deploy **${STACK_NAME}** to **${ENVIRONMENT}** (v$version) by ${GITHUB_ACTOR}",
            "url": "$build_url",
            "color": $([[ "$status" == "success" ]] && echo "65280" || [[ "$status" == "failure" ]] && echo "16711680" || echo "255"),
            "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%S.000Z)"
        }
    ]
}
EOF
)
    
    # Send Discord notification
    if curl -f -X POST \
        -H "Content-type: application/json" \
        --data "$discord_payload" \
        --max-time 30 \
        --silent \
        --show-error \
        "$webhook_url"; then
        
        log_info "✅ Discord notification sent successfully"
    else
        log_error "❌ Failed to send Discord notification"
    fi
}

main() {
    local status="${1:-started}"
    
    log_info "Processing notification for status: $status"
    
    # Send to both Slack and Discord
    send_slack_notification "$status" || log_warning "Slack notification failed"
    send_discord_notification "$status" || log_warning "Discord notification failed"
}

main "$@"

#!/bin/bash
# Fix ownership of mounted volumes (they may be owned by root from the host)
chown -R mcpanel:mcpanel /AdPanel/Servers /AdPanel/data /AdPanel/Backups 2>/dev/null

# Drop privileges and run the panel as mcpanel
exec gosu mcpanel /AdPanel/orexa-panel


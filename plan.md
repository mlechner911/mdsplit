📋 Projektplan: Markdown-Splitter für LLM-Übersetzung
🎯 Ziel
Ein Tool das Markdown-Dateien so aufteilt, dass:

Jeder Chunk ≤ X Zeichen ist (außer bei unteilbaren Blöcken)

Code-Blöcke immer komplett bleiben

HTML-Blöcke immer komplett bleiben (mit Stack-Parser)

Tabellen immer komplett bleiben

Listen immer komplett bleiben

Überschriften sind bevorzugte Split-Punkte

Kein Chunk wird unnötig groß (> 2x Zielgröße)


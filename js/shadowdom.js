// This function is executed in the context of the target page
const MIN_CONTENT_LENGTH = 50; // Minimum characters for "significant" content

const DEFAULT_EXCLUDE_TAGS = [
    'aisidebar-container',
    'template',
    'slot',
    'script',
    'style',
    'svg',
]

/**
 * @typedef {object} DeepCloneOptions
 * @property {string[]} [excludeClasses] - 需要排除的 CSS 类名数组。
 * @property {string[]} [excludeTags] - 需要排除的 HTML 标签名数组 (小写)。
 */

/**
 * 检查一个元素是否应该根据提供的选项被排除。
 * @param {Node} element - 需要检查的 DOM 节点。
 * @param {DeepCloneOptions} options - 包含排除规则的选项对象。
 * @returns {boolean} 如果元素应该被排除，则返回 true；否则返回 false。
 */
const shouldExclude = (element, options) => {
    // 仅处理 HTMLElement，其他类型的节点（如文本节点）不排除
    if (!(element instanceof HTMLElement)) {
        return false;
    }

    // 从 options 中解构出排除规则，并提供默认空数组
    const { excludeClasses = [], excludeTags = [] } = options;

    // 合并默认排除标签和用户自定义排除标签
    const mergedExcludeTags = [...DEFAULT_EXCLUDE_TAGS, ...excludeTags];

    // 如果元素的标签名在排除列表中，则返回 true
    if (mergedExcludeTags.includes(element.tagName.toLowerCase())) {
        return true;
    }

    // 检查元素是否包含任何一个需要排除的 class
    return excludeClasses.some((cls) => element.classList.contains(cls));
};

/**
 * 递归地克隆一个 DOM 节点，并特殊处理 Web Component 及其 Shadow DOM。
 * @param {Node} node - 需要克隆的 DOM 节点。
 * @param {number} level - 需要克隆的原始 Document 对象。
 * @param {DeepCloneOptions} [options={}] - 克隆选项，用于排除特定节点。
 * @returns {Node|undefined} 返回克隆后的节点。如果是被视为空的 Web Component，则返回 undefined。
 */
function deepCloneWithShadowDOM(node, level, options = {}) {
    const hasShadowRoot = node instanceof HTMLElement && node.shadowRoot && node.shadowRoot instanceof ShadowRoot;
    const isWebComponent = hasShadowRoot || (node instanceof HTMLElement && node.tagName.includes('-'));

    // const prefix = '-'.repeat(level*2);

    // 对 Web Component 使用 <div> 作为包裹，避免自定义元素行为冲突
    const clone = isWebComponent ? document.createElement('div') : node.cloneNode(false);

    if (hasShadowRoot) {
        // 递归克隆所有子节点
        node.shadowRoot.childNodes.forEach((child) => {
            if (shouldExclude(child, options)) {
                // 如果子节点需要被排除，则跳过
                // console.log(prefix, "shouldExclude", child);
                return;
            }

            const clonedChild = deepCloneWithShadowDOM(child, level + 1, options);

            if (!clonedChild) {
                // 如果克隆结果为空（比如空的 Web Component），则跳过
                // console.log(prefix, child, "clonedChild is nil");
                return;
            }

            clone.appendChild(clonedChild);
        });
    }

    // 递归克隆所有子节点
    node.childNodes.forEach((child) => {
        if (shouldExclude(child, options)) {
            // 如果子节点需要被排除，则跳过
            // console.log(prefix, "shouldExclude", child);
            return;
        }

        const clonedChild = deepCloneWithShadowDOM(child, level+1, options);

        if (!clonedChild) {
            // 如果克隆结果为空（比如空的 Web Component），则跳过
            // console.log(prefix, child, "clonedChild is nil");
            return;
        }

        clone.appendChild(clonedChild);
    });

    // 如果是一个 Web Component 克隆，并且最终没有任何内容，则不返回该节点
    if (isWebComponent && !clone.textContent?.trim()) {
        // console.log(prefix, clone, "isWebComponent and no text content");
        return undefined;
    }

    // if (node.nodeName !== "#text" && node.nodeName !== "#comment") {
    //     console.log(prefix, node, clone, "added");
    // }

    return clone;
}

/**
 * 深度克隆一个 Document 对象，同时处理 Shadow DOM。
 * @param {Document} doc - 需要克隆的原始 Document 对象。
 * @param {DeepCloneOptions} [options={}] - 克隆选项。
 * @returns {Document} 克隆后的新 Document 对象。
 */
function deepCloneDocumentWithShadowDOM(doc, options = {}) {
    const clonedDoc = document.implementation.createHTMLDocument();

    for (const child of doc.head.childNodes) {
        if (shouldExclude(child, options)) {
            // console.log("shouldExclude", child);
            continue;
        }
        const clonedChild = deepCloneWithShadowDOM(child, 1, options);
        if (clonedChild) {
            clonedDoc.head.appendChild(clonedChild);
        } else {
            // console.log(child, "deepCloneWithShadowDOM is nil");
        }
    }

    for (const child of doc.body.childNodes) {
        if (shouldExclude(child, options)) {
            // console.log("shouldExclude", child);
            continue;
        }
        const clonedChild = deepCloneWithShadowDOM(child, 1, options);
        if (clonedChild) {
            clonedDoc.body.appendChild(clonedChild);
            // console.log("clonedChild:",clonedChild);
        } else {
            // console.log(child, "deepCloneWithShadowDOM is nil");
        }
    }

    // console.log(clonedDoc);
    return clonedDoc;
}

// const BRAND_ID = 'aisidebar';
// const TRANSLATOR_ID = `${BRAND_ID}-translator`;
// const translationTargetClass = `${TRANSLATOR_ID}--translation-target`;
// const translationTargetDividerClass = `${TRANSLATOR_ID}--translation-target-divider`;
// const translationTargetInnerClass = `${TRANSLATOR_ID}--translation-target-inner`;
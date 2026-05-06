import { BrowserUseError } from "./errors.js";

/**
 * 提取页面的完整数据内容。
 * @param {import('playwright').Page} page
 * @returns {Promise<object>}
 */
export async function extractPageData(page) {
  return page.evaluate(() => {
    /**
     * 将 NamedNodeMap 转为普通对象。
     * @param {Element} element
     * @returns {Record<string, string>}
     */
    function getAttributes(element) {
      const result = {};
      for (const attribute of Array.from(element.attributes || [])) {
        result[attribute.name] = attribute.value;
      }
      return result;
    }

    /**
     * 获取元素文本。
     * @param {Element | null} element
     * @returns {string}
     */
    function getText(element) {
      return (element?.textContent || "").replace(/\s+/g, " ").trim();
    }

    /**
     * 判断元素是否含 data-* 属性。
     * @param {Element} element
     * @returns {boolean}
     */
    function hasDataAttributes(element) {
      return Array.from(element.attributes || []).some((attribute) => attribute.name.startsWith("data-"));
    }

    /**
     * 提取基础页面信息。
     * @returns {object}
     */
    function extractPageMeta() {
      const metas = Array.from(document.querySelectorAll("meta")).map((node) => ({
        name: node.getAttribute("name"),
        property: node.getAttribute("property"),
        content: node.getAttribute("content"),
      }));
      const canonical = document.querySelector('link[rel="canonical"]')?.getAttribute("href") || "";
      return {
        title: document.title,
        url: window.location.href,
        lang: document.documentElement.lang || "",
        canonical,
        meta: metas,
      };
    }

    /**
     * 提取结构化数据。
     * @returns {object}
     */
    function extractStructured() {
      const jsonLd = Array.from(document.querySelectorAll('script[type="application/ld+json"]')).map((node) => node.textContent || "");
      const openGraph = {};
      const twitterCard = {};

      for (const meta of Array.from(document.querySelectorAll("meta[property], meta[name]"))) {
        const property = meta.getAttribute("property") || "";
        const name = meta.getAttribute("name") || "";
        const content = meta.getAttribute("content") || "";
        if (property.startsWith("og:")) {
          openGraph[property] = content;
        }
        if (name.startsWith("twitter:")) {
          twitterCard[name] = content;
        }
      }

      const microdata = Array.from(document.querySelectorAll("[itemscope]")).map((node) => ({
        itemType: node.getAttribute("itemtype") || "",
        itemId: node.getAttribute("itemid") || "",
        text: getText(node),
      }));

      return { jsonLd, openGraph, twitterCard, microdata };
    }

    /**
     * 提取表单信息。
     * @returns {object[]}
     */
    function extractForms() {
      return Array.from(document.forms).map((form, index) => ({
        index,
        id: form.id || "",
        name: form.getAttribute("name") || "",
        method: form.getAttribute("method") || "GET",
        action: form.getAttribute("action") || "",
        fields: Array.from(form.elements).map((field) => ({
          tag: field.tagName,
          type: field.getAttribute?.("type") || "",
          name: field.getAttribute?.("name") || "",
          id: field.getAttribute?.("id") || "",
          value: "value" in field ? field.value : "",
          placeholder: field.getAttribute?.("placeholder") || "",
          required: Boolean(field.required),
          disabled: Boolean(field.disabled),
          checked: "checked" in field ? Boolean(field.checked) : false,
          text: getText(field),
          attributes: getAttributes(field),
        })),
      }));
    }

    /**
     * 提取表格数据。
     * @returns {object[]}
     */
    function extractTables() {
      return Array.from(document.querySelectorAll("table")).map((table, index) => ({
        index,
        caption: getText(table.querySelector("caption")),
        headers: Array.from(table.querySelectorAll("thead th")).map((node) => getText(node)),
        rows: Array.from(table.querySelectorAll("tr")).map((row) =>
          Array.from(row.querySelectorAll("th, td")).map((cell) => getText(cell)),
        ),
      }));
    }

    /**
     * 提取列表数据。
     * @returns {object[]}
     */
    function extractLists() {
      return Array.from(document.querySelectorAll("ul, ol")).map((list, index) => ({
        index,
        tag: list.tagName.toLowerCase(),
        items: Array.from(list.querySelectorAll(":scope > li")).map((item) => getText(item)),
      }));
    }

    /**
     * 提取链接数据。
     * @returns {object[]}
     */
    function extractLinks() {
      return Array.from(document.querySelectorAll("a[href]")).map((link) => ({
        text: getText(link),
        href: link.href,
        attributes: getAttributes(link),
      }));
    }

    /**
     * 提取媒体数据。
     * @returns {object[]}
     */
    function extractMedia() {
      return Array.from(document.querySelectorAll("img, video, audio, source")).map((node) => ({
        tag: node.tagName.toLowerCase(),
        src: node.getAttribute("src") || "",
        currentSrc: "currentSrc" in node ? node.currentSrc || "" : "",
        alt: node.getAttribute("alt") || "",
        attributes: getAttributes(node),
      }));
    }

    /**
     * 提取属性样本。
     * @returns {object[]}
     */
    function extractAttributes() {
      return Array.from(document.querySelectorAll("*"))
        .filter((node) => {
          return Boolean(node.id)
            || Boolean(node.getAttribute("class"))
            || Boolean(node.getAttribute("name"))
            || Boolean(node.getAttribute("role"))
            || Boolean(node.getAttribute("aria-label"))
            || Boolean(node.getAttribute("placeholder"))
            || hasDataAttributes(node);
        })
        .slice(0, 500)
        .map((node) => ({
          tag: node.tagName.toLowerCase(),
          text: getText(node).slice(0, 200),
          attributes: getAttributes(node),
        }));
    }

    /**
     * 提取主要文本块。
     * @returns {object}
     */
    function extractText() {
      const headings = Array.from(document.querySelectorAll("h1, h2, h3, h4, h5, h6")).map((node) => ({
        tag: node.tagName.toLowerCase(),
        text: getText(node),
      }));
      const paragraphs = Array.from(document.querySelectorAll("p")).map((node) => getText(node)).filter(Boolean);
      const buttons = Array.from(document.querySelectorAll("button, input[type='button'], input[type='submit']")).map((node) => getText(node) || node.getAttribute("value") || "");
      const fullText = getText(document.body);
      return { headings, paragraphs, buttons, fullText };
    }

    /**
     * 提取脚本中内嵌 JSON 数据。
     * @returns {object[]}
     */
    function extractEmbeddedData() {
      return Array.from(document.querySelectorAll("script"))
        .map((node, index) => ({ index, text: (node.textContent || "").trim() }))
        .filter((item) => item.text.startsWith("{") || item.text.startsWith("["))
        .slice(0, 50);
    }

    /**
     * 提取 DOM 结构摘要。
     * @returns {object}
     */
    function extractStructure() {
      return {
        forms: document.forms.length,
        tables: document.querySelectorAll("table").length,
        lists: document.querySelectorAll("ul, ol").length,
        links: document.querySelectorAll("a[href]").length,
        images: document.querySelectorAll("img").length,
        sections: Array.from(document.querySelectorAll("main, section, article, aside, nav, header, footer")).map((node) => ({
          tag: node.tagName.toLowerCase(),
          id: node.id || "",
          className: node.className || "",
          text: getText(node).slice(0, 200),
        })),
      };
    }

    return {
      page: extractPageMeta(),
      text: extractText(),
      structured: extractStructured(),
      forms: extractForms(),
      tables: extractTables(),
      lists: extractLists(),
      links: extractLinks(),
      media: extractMedia(),
      attributes: extractAttributes(),
      embeddedData: extractEmbeddedData(),
      structure: extractStructure(),
    };
  });
}

/**
 * 安全执行提取动作并转换异常。
 * @param {import('playwright').Page} page
 * @returns {Promise<object>}
 */
export async function safeExtractPageData(page) {
  try {
    return await extractPageData(page);
  } catch (error) {
    throw new BrowserUseError("EXTRACTION_FAILED", "页面数据提取失败", {
      stage: "extract",
      cause: error,
    });
  }
}

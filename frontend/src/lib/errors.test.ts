import {describe,it,expect} from "vitest";
import {ApiError,errorText} from "./api";
describe("human readable errors",()=>{
 it("translates known codes and legacy English responses",()=>{
  expect(errorText(new ApiError(404,"session not found"))).toContain("Сессия уже закончена");
  expect(errorText(new ApiError(409,"Not enough coins",{code:"INSUFFICIENT_COINS"}))).toContain("Не хватает монет");
 });
 it("keeps precise field advice but hides internal errors",()=>{
  expect(errorText(new ApiError(400,"Дата выбрана — осталось указать время"))).toBe("Дата выбрана — осталось указать время");
  expect(errorText(new ApiError(500,"SQL connection failed"))).not.toContain("SQL");
  expect(errorText(new TypeError("Failed to fetch"))).toContain("Нет связи");
 });
});
